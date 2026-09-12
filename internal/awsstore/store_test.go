package awsstore

import (
	"errors"
	"testing"

	localstore "github.com/agent-surface/agent-surface/internal/store"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDeleteRecordsInputRequiresCurrentRevision(t *testing.T) {
	v := localstore.Surface{ID: "surface", PublicID: "public"}
	v.Result.Revision = 42
	input := deleteRecordsInput("table", v)
	if len(input.TransactItems) != 2 {
		t.Fatalf("items=%d, want 2", len(input.TransactItems))
	}
	canonical := input.TransactItems[0].Delete
	if got := aws.ToString(canonical.ConditionExpression); got != "revision = :revision" {
		t.Fatalf("condition=%q", got)
	}
	revision, ok := canonical.ExpressionAttributeValues[":revision"].(*types.AttributeValueMemberN)
	if !ok || revision.Value != "42" {
		t.Fatalf("revision=%#v, want 42", canonical.ExpressionAttributeValues[":revision"])
	}
}

func TestIsDeleteConflict(t *testing.T) {
	conditional := &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}}}
	if !isDeleteConflict(conditional) {
		t.Fatal("conditional cancellation was not classified as a conflict")
	}
	if !isDeleteConflict(&types.TransactionConflictException{}) {
		t.Fatal("transaction conflict was not classified as a conflict")
	}
	if isDeleteConflict(errors.New("network failure")) {
		t.Fatal("unrelated error was classified as a conflict")
	}
}
