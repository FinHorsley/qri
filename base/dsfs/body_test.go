package dsfs

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"
)

func TestLoadBody(t *testing.T) {
	ctx := context.Background()
	datasets, fs, err := makeFilestore()
	if err != nil {
		t.Fatalf("error creating test filestore: %s", err.Error())
	}

	ds, err := LoadDataset(ctx, fs, datasets["cities"])
	if err != nil {
		t.Fatalf("error loading dataset: %s", err.Error())
	}

	f, err := LoadBody(ctx, fs, ds)
	if err != nil {
		t.Fatalf("error loading data: %s", err.Error())
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatalf("error reading data file: %s", err.Error())
	}

	eq, err := ioutil.ReadFile("testdata/cities/body.csv")
	if err != nil {
		t.Fatalf("error reading test file: %s", err.Error())
	}

	if !bytes.Equal(data, eq) {
		t.Errorf("byte mismatch. expected: %s, got: %s", string(eq), string(data))
	}
}
