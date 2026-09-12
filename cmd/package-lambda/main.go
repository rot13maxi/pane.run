// Command package-lambda creates a Lambda zip with an executable bootstrap.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: package-lambda INPUT OUTPUT")
		os.Exit(2)
	}
	input, err := os.Open(os.Args[1])
	if err != nil {
		fatal(err)
	}
	defer input.Close()
	output, err := os.Create(os.Args[2])
	if err != nil {
		fatal(err)
	}
	zw := zip.NewWriter(output)
	header := &zip.FileHeader{Name: "bootstrap", Method: zip.Deflate}
	header.SetMode(0755)
	entry, err := zw.CreateHeader(header)
	if err != nil {
		fatal(err)
	}
	if _, err = io.Copy(entry, input); err != nil {
		fatal(err)
	}
	if err = zw.Close(); err != nil {
		fatal(err)
	}
	if err = output.Close(); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
