package awsstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agent-surface/agent-surface/internal/schema"
	localstore "github.com/agent-surface/agent-surface/internal/store"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Store struct {
	db      *dynamodb.Client
	objects *s3.Client
	table   string
	bucket  string
	now     func() time.Time
}

type item struct {
	PK        string `dynamodbav:"pk"`
	Kind      string `dynamodbav:"kind"`
	Surface   []byte `dynamodbav:"surface,omitempty"`
	SurfaceID string `dynamodbav:"surface_id,omitempty"`
	Revision  uint64 `dynamodbav:"revision"`
	Expires   int64  `dynamodbav:"expires_at"`
}

func New(db *dynamodb.Client, objects *s3.Client, table, bucket string) (*Store, error) {
	if db == nil || objects == nil || table == "" || bucket == "" {
		return nil, errors.New("dynamodb client, s3 client, table, and bucket are required")
	}
	return &Store{db: db, objects: objects, table: table, bucket: bucket, now: time.Now}, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func hash(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func authorized(s localstore.Surface, capability string) bool {
	a, b := []byte(s.ManagementHash), []byte(hash(capability))
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}
func clone(s localstore.Surface) localstore.Surface {
	b, _ := json.Marshal(s)
	var out localstore.Surface
	_ = json.Unmarshal(b, &out)
	return out
}
func surfaceKey(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "SURFACE#" + id}}
}
func publicKey(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "PUBLIC#" + id}}
}

func (s *Store) Create(spec schema.Spec, ttl time.Duration) (localstore.Surface, string, error) {
	return s.CreateWithValues(spec, ttl, nil)
}

func (s *Store) CreateWithValues(spec schema.Spec, ttl time.Duration, values map[string]any) (localstore.Surface, string, error) {
	if values == nil {
		values = map[string]any{}
	}
	if err := schema.ValidateValues(spec, values); err != nil {
		return localstore.Surface{}, "", err
	}
	id, err := randomToken(18)
	if err != nil {
		return localstore.Surface{}, "", err
	}
	publicID, err := randomToken(18)
	if err != nil {
		return localstore.Surface{}, "", err
	}
	capability, err := randomToken(32)
	if err != nil {
		return localstore.Surface{}, "", err
	}
	now := s.now().UTC()
	v := localstore.Surface{ID: id, PublicID: publicID, ManagementHash: hash(capability), Spec: spec, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	v.Result = schema.Result{Status: schema.StatusActive, Values: values, CreatedAt: now, UpdatedAt: now, ExpiresAt: v.ExpiresAt}
	canonical, err := marshalItem(v)
	if err != nil {
		return localstore.Surface{}, "", err
	}
	mapping, err := attributevalue.MarshalMap(item{PK: "PUBLIC#" + publicID, Kind: "public", SurfaceID: id, Expires: v.ExpiresAt.Unix()})
	if err != nil {
		return localstore.Surface{}, "", err
	}
	_, err = s.db.TransactWriteItems(context.Background(), &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(s.table), Item: canonical, ConditionExpression: aws.String("attribute_not_exists(pk)")}},
		{Put: &types.Put{TableName: aws.String(s.table), Item: mapping, ConditionExpression: aws.String("attribute_not_exists(pk)")}},
	}})
	if err != nil {
		return localstore.Surface{}, "", fmt.Errorf("create surface: %w", err)
	}
	return clone(v), capability, nil
}

func marshalItem(v localstore.Surface) (map[string]types.AttributeValue, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return attributevalue.MarshalMap(item{PK: "SURFACE#" + v.ID, Kind: "surface", Surface: b, Revision: v.Result.Revision, Expires: v.ExpiresAt.Unix()})
}
func (s *Store) loadKey(key map[string]types.AttributeValue) (localstore.Surface, error) {
	out, err := s.db.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(s.table), Key: key, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return localstore.Surface{}, err
	}
	if len(out.Item) == 0 {
		return localstore.Surface{}, localstore.ErrNotFound
	}
	var row item
	if err = attributevalue.UnmarshalMap(out.Item, &row); err != nil {
		return localstore.Surface{}, err
	}
	if row.Kind == "public" {
		return s.loadKey(surfaceKey(row.SurfaceID))
	}
	var v localstore.Surface
	if err = json.Unmarshal(row.Surface, &v); err != nil {
		return v, err
	}
	return v, nil
}
func (s *Store) live(v localstore.Surface) error {
	if !s.now().Before(v.ExpiresAt) {
		return localstore.ErrExpired
	}
	return nil
}
func (s *Store) Get(id, capability string) (localstore.Surface, error) {
	v, e := s.loadKey(surfaceKey(id))
	if e != nil {
		return v, e
	}
	if !authorized(v, capability) {
		return v, localstore.ErrForbidden
	}
	if e = s.live(v); e != nil {
		return v, e
	}
	return clone(v), nil
}
func (s *Store) Public(id string) (localstore.Surface, error) {
	v, e := s.loadKey(publicKey(id))
	if e != nil {
		return v, e
	}
	if e = s.live(v); e != nil {
		return v, e
	}
	return clone(v), nil
}

func (s *Store) save(v localstore.Surface, oldRevision uint64) error {
	row, err := marshalItem(v)
	if err != nil {
		return err
	}
	_, err = s.db.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(s.table), Item: row, ConditionExpression: aws.String("revision = :revision"), ExpressionAttributeValues: map[string]types.AttributeValue{":revision": &types.AttributeValueMemberN{Value: fmt.Sprint(oldRevision)}}})
	var conflict *types.ConditionalCheckFailedException
	if errors.As(err, &conflict) {
		return localstore.ErrConflict
	}
	return err
}
func (s *Store) Update(id, capability string, spec schema.Spec) (localstore.Surface, error) {
	v, e := s.Get(id, capability)
	if e != nil {
		return v, e
	}
	old := v.Result.Revision
	v.Result.Values = schema.FilterCompatibleValues(v.Spec, spec, v.Result.Values)
	v.Spec = spec
	v.Result.Revision++
	v.Result.UpdatedAt = s.now().UTC()
	if e = s.save(v, old); e != nil {
		return localstore.Surface{}, e
	}
	return clone(v), nil
}
func (s *Store) WriteState(publicID string, revision uint64, values map[string]any) (localstore.Surface, error) {
	v, e := s.Public(publicID)
	if e != nil {
		return v, e
	}
	if v.ClosedAt != nil {
		return v, localstore.ErrClosed
	}
	if revision != v.Result.Revision {
		return v, localstore.ErrConflict
	}
	if e = schema.ValidateValues(v.Spec, values); e != nil {
		return v, e
	}
	old := v.Result.Revision
	v.Result.Values = values
	v.Result.Revision++
	v.Result.UpdatedAt = s.now().UTC()
	if e = s.save(v, old); e != nil {
		if errors.Is(e, localstore.ErrConflict) {
			current, _ := s.Public(publicID)
			return current, e
		}
		return v, e
	}
	return clone(v), nil
}
func (s *Store) Submit(publicID string) (localstore.Surface, error) {
	v, e := s.Public(publicID)
	if e != nil {
		return v, e
	}
	if v.ClosedAt != nil {
		return v, localstore.ErrClosed
	}
	if e = schema.ValidateSubmission(v.Spec, v.Result.Values); e != nil {
		return v, e
	}
	old := v.Result.Revision
	now := s.now().UTC()
	v.Result.Status = schema.StatusSubmitted
	v.Result.SubmittedAt = &now
	v.Result.Revision++
	v.Result.UpdatedAt = now
	if e = s.save(v, old); e != nil {
		return v, e
	}
	return clone(v), nil
}
func (s *Store) Reset(publicID string) (localstore.Surface, error) {
	v, e := s.Public(publicID)
	if e != nil {
		return v, e
	}
	if v.ClosedAt != nil {
		return v, localstore.ErrClosed
	}
	old := v.Result.Revision
	v.Result.Status = schema.StatusActive
	v.Result.Values = map[string]any{}
	v.Result.SubmittedAt = nil
	v.Result.Revision++
	v.Result.UpdatedAt = s.now().UTC()
	if e = s.save(v, old); e != nil {
		return v, e
	}
	return clone(v), nil
}
func (s *Store) Close(id, capability string) (localstore.Surface, error) {
	v, e := s.Get(id, capability)
	if e != nil {
		return v, e
	}
	if v.ClosedAt != nil {
		return v, nil
	}
	old := v.Result.Revision
	now := s.now().UTC()
	v.ClosedAt = &now
	v.Result.Status = schema.StatusClosed
	v.Result.Revision++
	v.Result.UpdatedAt = now
	if e = s.save(v, old); e != nil {
		return v, e
	}
	return clone(v), nil
}
func (s *Store) Delete(id, capability string) error {
	v, e := s.Get(id, capability)
	if e != nil {
		return e
	}
	_, e = s.db.TransactWriteItems(context.Background(), &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{{Delete: &types.Delete{TableName: aws.String(s.table), Key: surfaceKey(id)}}, {Delete: &types.Delete{TableName: aws.String(s.table), Key: publicKey(v.PublicID)}}}})
	if e != nil {
		return e
	}
	for _, a := range v.Assets {
		_, _ = s.objects.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String("a/" + v.PublicID + "/" + a.ID)})
	}
	return nil
}
func (s *Store) AddAsset(id, capability, filename, contentType string, data []byte) (localstore.Surface, localstore.Asset, error) {
	v, e := s.Get(id, capability)
	if e != nil {
		return v, localstore.Asset{}, e
	}
	assetID, e := randomToken(18)
	if e != nil {
		return v, localstore.Asset{}, e
	}
	key := "a/" + v.PublicID + "/" + assetID
	_, e = s.objects.PutObject(context.Background(), &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String(contentType), ContentDisposition: aws.String("inline"), CacheControl: aws.String("no-store")})
	if e != nil {
		return v, localstore.Asset{}, e
	}
	a := localstore.Asset{ID: assetID, Filename: filename, ContentType: contentType, Size: int64(len(data))}
	old := v.Result.Revision
	v.Assets = append(v.Assets, a)
	v.Result.Revision++
	v.Result.UpdatedAt = s.now().UTC()
	if e = s.save(v, old); e != nil {
		_, _ = s.objects.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
		return v, a, e
	}
	return clone(v), a, nil
}
func (s *Store) Asset(string, string) (localstore.Asset, string, error) {
	return localstore.Asset{}, "", localstore.ErrNotFound
}
func (s *Store) PutPage(v localstore.Surface, html []byte) error {
	_, e := s.objects.PutObject(context.Background(), &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String("s/" + v.PublicID), Body: bytes.NewReader(html), ContentType: aws.String("text/html; charset=utf-8"), CacheControl: aws.String("no-store")})
	return e
}
func (s *Store) DeletePage(publicID string) error {
	_, e := s.objects.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String("s/" + publicID)})
	return e
}
