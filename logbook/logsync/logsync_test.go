package logsync

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	golog "github.com/ipfs/go-log"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/qri-io/dataset"
	"github.com/qri-io/qfs"
	testPeers "github.com/qri-io/qri/config/test"
	"github.com/qri-io/qri/dsref"
	"github.com/qri-io/qri/event"
	"github.com/qri-io/qri/logbook"
	"github.com/qri-io/qri/logbook/oplog"
	"github.com/qri-io/qri/profile"
)

func Example() {
	// first some boilerplate setup
	ctx, done := context.WithCancel(context.Background())
	defer done()

	// our example has two authors. Johnathon and Basit are going to sync logbooks
	// let's start with two empty logbooks
	johnathonsLogbook := makeJohnathonLogbook()
	basitsLogbook := makeBasitLogbook()

	wait := make(chan struct{}, 1)

	// create a logsync from basit's logbook:
	basitLogsync := New(basitsLogbook, func(o *Options) {
		// we MUST override the PreCheck function. In this example we're only going
		// to allow pushes from johnathon
		o.PushPreCheck = func(ctx context.Context, author profile.Author, ref dsref.Ref, l *oplog.Log) error {
			if author.AuthorID() != johnathonsLogbook.Author().AuthorID() {
				return fmt.Errorf("rejected for secret reasons")
			}
			return nil
		}

		o.Pushed = func(ctx context.Context, author profile.Author, ref dsref.Ref, l *oplog.Log) error {
			wait <- struct{}{}
			return nil
		}
	})

	// for this example we're going to do sync over HTTP.
	// create an HTTP handler for the remote & wire it up to an example server
	handleFunc := HTTPHandler(basitLogsync)
	server := httptest.NewServer(handleFunc)
	defer server.Close()

	// johnathon creates a dataset with a bunch of history:
	worldBankDatasetRef := makeWorldBankLogs(ctx, johnathonsLogbook)

	items, err := johnathonsLogbook.Items(ctx, worldBankDatasetRef, 0, 100)
	if err != nil {
		panic(err)
	}
	fmt.Printf("johnathon has %d references for %s\n", len(items), worldBankDatasetRef.Human())

	// johnathon creates a new push
	johnathonLogsync := New(johnathonsLogbook)
	push, err := johnathonLogsync.NewPush(worldBankDatasetRef, server.URL)
	if err != nil {
		panic(err)
	}

	// execute the push, sending jonathon's world bank reference to basit
	if err = push.Do(ctx); err != nil {
		panic(err)
	}

	// wait for sync to complete
	<-wait
	if items, err = basitsLogbook.Items(ctx, worldBankDatasetRef, 0, 100); err != nil {
		panic(err)
	}
	fmt.Printf("basit has %d references for %s\n", len(items), worldBankDatasetRef.Human())

	// this time basit creates a history
	nasdaqDatasetRef := makeNasdaqLogs(ctx, basitsLogbook)

	if items, err = basitsLogbook.Items(ctx, nasdaqDatasetRef, 0, 100); err != nil {
		panic(err)
	}
	fmt.Printf("basit has %d references for %s\n", len(items), nasdaqDatasetRef.Human())

	// prepare to pull nasdaq refs from basit
	pull, err := johnathonLogsync.NewPull(nasdaqDatasetRef, server.URL)
	if err != nil {
		panic(err)
	}
	// setting merge=true will persist logs to the logbook if the pull succeeds
	pull.Merge = true

	if _, err = pull.Do(ctx); err != nil {
		panic(err)
	}

	if items, err = johnathonsLogbook.Items(ctx, nasdaqDatasetRef, 0, 100); err != nil {
		panic(err)
	}
	fmt.Printf("johnathon has %d references for %s\n", len(items), nasdaqDatasetRef.Human())

	// Output: johnathon has 3 references for johnathon/world_bank_population
	// basit has 3 references for johnathon/world_bank_population
	// basit has 2 references for basit/nasdaq
	// johnathon has 2 references for basit/nasdaq
}

func TestHookCalls(t *testing.T) {
	tr, cleanup := newTestRunner(t)
	defer cleanup()

	hooksCalled := []string{}
	callCheck := func(s string) Hook {
		return func(ctx context.Context, a profile.Author, ref dsref.Ref, l *oplog.Log) error {
			hooksCalled = append(hooksCalled, s)
			return nil
		}
	}

	nasdaqRef, err := writeNasdaqLogs(tr.Ctx, tr.A)
	if err != nil {
		t.Fatal(err)
	}

	lsA := New(tr.A, func(o *Options) {
		o.PullPreCheck = callCheck("PullPreCheck")
		o.Pulled = callCheck("Pulled")
		o.PushPreCheck = callCheck("PushPreCheck")
		o.PushFinalCheck = callCheck("PushFinalCheck")
		o.Pushed = callCheck("Pushed")
		o.RemovePreCheck = callCheck("RemovePreCheck")
		o.Removed = callCheck("Removed")
	})

	s := httptest.NewServer(HTTPHandler(lsA))
	defer s.Close()

	lsB := New(tr.B)

	pull, err := lsB.NewPull(nasdaqRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	pull.Merge = true

	if _, err := pull.Do(tr.Ctx); err != nil {
		t.Fatal(err)
	}

	worldBankRef, err := writeWorldBankLogs(tr.Ctx, tr.B)
	if err != nil {
		t.Fatal(err)
	}
	push, err := lsB.NewPush(worldBankRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := push.Do(tr.Ctx); err != nil {
		t.Fatal(err)
	}

	if err := lsB.DoRemove(tr.Ctx, worldBankRef, s.URL); err != nil {
		t.Fatal(err)
	}

	expectHooksCallOrder := []string{
		"PullPreCheck",
		"Pulled",
		"PushPreCheck",
		"PushFinalCheck",
		"Pushed",
		"RemovePreCheck",
		"Removed",
	}

	if diff := cmp.Diff(expectHooksCallOrder, hooksCalled); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestHookErrors(t *testing.T) {
	tr, cleanup := newTestRunner(t)
	defer cleanup()

	worldBankRef, err := writeWorldBankLogs(tr.Ctx, tr.B)
	if err != nil {
		t.Fatal(err)
	}

	hooksCalled := []string{}
	callCheck := func(s string) Hook {
		return func(ctx context.Context, a profile.Author, ref dsref.Ref, l *oplog.Log) error {
			hooksCalled = append(hooksCalled, s)
			return fmt.Errorf("hook failed")
		}
	}

	nasdaqRef, err := writeNasdaqLogs(tr.Ctx, tr.A)
	if err != nil {
		t.Fatal(err)
	}

	lsA := New(tr.A, func(o *Options) {
		o.PullPreCheck = callCheck("PullPreCheck")
		o.PushPreCheck = callCheck("PushPreCheck")
		o.RemovePreCheck = callCheck("RemovePreCheck")

		o.PushFinalCheck = callCheck("PushFinalCheck")

		o.Pulled = callCheck("Pulled")
		o.Pushed = callCheck("Pushed")
		o.Removed = callCheck("Removed")
	})

	s := httptest.NewServer(HTTPHandler(lsA))
	defer s.Close()

	lsB := New(tr.B)

	pull, err := lsB.NewPull(nasdaqRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	pull.Merge = true

	if _, err := pull.Do(tr.Ctx); err == nil {
		t.Fatal(err)
	}
	push, err := lsB.NewPush(worldBankRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := push.Do(tr.Ctx); err == nil {
		t.Fatal(err)
	}
	if err := lsB.DoRemove(tr.Ctx, worldBankRef, s.URL); err == nil {
		t.Fatal(err)
	}

	lsA.pushPreCheck = nil
	lsA.pullPreCheck = nil
	lsA.removePreCheck = nil

	push, err = lsB.NewPush(worldBankRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := push.Do(tr.Ctx); err == nil {
		t.Fatal(err)
	}

	lsA.pushFinalCheck = nil

	pull, err = lsB.NewPull(nasdaqRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	pull.Merge = true

	if _, err := pull.Do(tr.Ctx); err != nil {
		t.Fatal(err)
	}
	push, err = lsB.NewPush(worldBankRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = push.Do(tr.Ctx); err != nil {
		t.Fatal(err)
	}
	if err := lsB.DoRemove(tr.Ctx, worldBankRef, s.URL); err != nil {
		t.Fatal(err)
	}

	expectHooksCallOrder := []string{
		"PullPreCheck",
		"PushPreCheck",
		"RemovePreCheck",

		"PushFinalCheck",

		"Pulled",
		"Pushed",
		"Removed",
	}

	if diff := cmp.Diff(expectHooksCallOrder, hooksCalled); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestWrongProfileID(t *testing.T) {
	tr, cleanup := newTestRunner(t)
	defer cleanup()

	worldBankRef, err := writeWorldBankLogs(tr.Ctx, tr.B)
	if err != nil {
		t.Fatal(err)
	}

	nasdaqRef, err := writeNasdaqLogs(tr.Ctx, tr.A)
	if err != nil {
		t.Fatal(err)
	}

	// Modify the profileID of this reference, which should cause it to fail to push
	worldBankRef.ProfileID = testPeers.GetTestPeerInfo(1).EncodedPeerID

	lsA := New(tr.A)

	s := httptest.NewServer(HTTPHandler(lsA))
	defer s.Close()

	lsB := New(tr.B)
	pull, err := lsB.NewPull(nasdaqRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	pull.Merge = true
	if _, err := pull.Do(tr.Ctx); err != nil {
		t.Fatal(err)
	}

	// B tries to push, but the profileID it uses has been modifed to something else
	// Logsync will catch this error.
	push, err := lsB.NewPush(worldBankRef, s.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = push.Do(tr.Ctx)
	if err == nil {
		t.Errorf("expected error but did not get one")
	}
	expectErr := `ref contained in log data does not match`
	if expectErr != err.Error() {
		t.Errorf("error mismatch, expect: %s, got: %s", expectErr, err)
	}
}

func TestNilCallable(t *testing.T) {
	var logsync *Logsync

	if a := logsync.Author(); a != nil {
		t.Errorf("author mismatch. expected: '%v', got: '%v' ", nil, a)
	}

	if _, err := logsync.NewPush(dsref.Ref{}, ""); err != ErrNoLogsync {
		t.Errorf("error mismatch. expected: '%v', got: '%v' ", ErrNoLogsync, err)
	}
	if _, err := logsync.NewPull(dsref.Ref{}, ""); err != ErrNoLogsync {
		t.Errorf("error mismatch. expected: '%v', got: '%v' ", ErrNoLogsync, err)
	}
	if err := logsync.DoRemove(context.Background(), dsref.Ref{}, ""); err != ErrNoLogsync {
		t.Errorf("error mismatch. expected: '%v', got: '%v' ", ErrNoLogsync, err)
	}
}

func makeJohnathonLogbook() *logbook.Book {
	var aPk = testPeers.GetTestPeerInfo(10).EncodedPrivKey

	pk, err := decodePk(aPk)
	if err != nil {
		panic(err)
	}

	book, err := newTestbook("johnathon", pk)
	if err != nil {
		panic(err)
	}
	return book
}

func makeBasitLogbook() *logbook.Book {
	var bPk = testPeers.GetTestPeerInfo(9).EncodedPrivKey

	pk, err := decodePk(bPk)
	if err != nil {
		panic(err)
	}

	book, err := newTestbook("basit", pk)
	if err != nil {
		panic(err)
	}
	return book
}

func makeWorldBankLogs(ctx context.Context, book *logbook.Book) dsref.Ref {
	ref, err := writeWorldBankLogs(ctx, book)
	if err != nil {
		panic(err)
	}
	return ref
}

func makeNasdaqLogs(ctx context.Context, book *logbook.Book) dsref.Ref {
	ref, err := writeNasdaqLogs(ctx, book)
	if err != nil {
		panic(err)
	}
	return ref
}

type testRunner struct {
	Ctx                context.Context
	A, B               *logbook.Book
	APrivKey, BPrivKey crypto.PrivKey
}

func (tr *testRunner) DefaultLogsyncs() (a, b *Logsync) {
	return New(tr.A), New(tr.B)
}

func newTestRunner(t *testing.T) (tr *testRunner, cleanup func()) {
	var aPk = testPeers.GetTestPeerInfo(10).EncodedPrivKey
	var bPk = testPeers.GetTestPeerInfo(9).EncodedPrivKey

	var err error
	tr = &testRunner{
		Ctx: context.Background(),
	}

	tr.APrivKey, err = decodePk(aPk)
	if err != nil {
		t.Fatal(err)
	}
	if tr.A, err = newTestbook("a", tr.APrivKey); err != nil {
		t.Fatal(err)
	}

	tr.BPrivKey, err = decodePk(bPk)
	if err != nil {
		t.Fatal(err)
	}
	if tr.B, err = newTestbook("b", tr.BPrivKey); err != nil {
		t.Fatal(err)
	}

	golog.SetLogLevel("logsync", "CRITICAL")
	cleanup = func() {
		golog.SetLogLevel("logsync", "ERROR")
	}
	return tr, cleanup
}

func decodePk(b64pk string) (crypto.PrivKey, error) {
	// logbooks are encrypted at rest, we need a private key to interact with
	// them, including to create a new logbook. This is a dummy Private Key
	// you should never, ever use in real life. demo only folks.
	data, err := base64.StdEncoding.DecodeString(b64pk)
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshalPrivateKey(data)
}

func newTestbook(username string, pk crypto.PrivKey) (*logbook.Book, error) {
	// logbook relies on a qfs.Filesystem for read & write. create an in-memory
	// filesystem we can play with
	fs := qfs.NewMemFS()
	return logbook.NewJournal(pk, username, event.NilBus, fs, "/mem/logbook.qfb")
}

func writeNasdaqLogs(ctx context.Context, book *logbook.Book) (ref dsref.Ref, err error) {
	name := "nasdaq"
	initID, err := book.WriteDatasetInit(ctx, name)
	if err != nil {
		return ref, err
	}

	ds := &dataset.Dataset{
		Peername: book.Username(),
		Name:     name,
		Commit: &dataset.Commit{
			Timestamp: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC),
			Title:     "init dataset",
		},
		Path:         "v0",
		PreviousPath: "",
	}

	if err = book.WriteVersionSave(ctx, initID, ds, nil); err != nil {
		return ref, err
	}

	ds.Path = "v1"
	ds.PreviousPath = "v0"

	if err = book.WriteVersionSave(ctx, initID, ds, nil); err != nil {
		return ref, err
	}

	return dsref.Ref{
		Username: book.Username(),
		Name:     name,
		InitID:   initID,
	}, nil
}

func writeWorldBankLogs(ctx context.Context, book *logbook.Book) (ref dsref.Ref, err error) {
	name := "world_bank_population"
	peerID, err := book.ActivePeerID(ctx)
	if err != nil {
		return dsref.Ref{}, err
	}

	initID, err := book.WriteDatasetInit(ctx, name)
	if err != nil {
		return ref, err
	}

	ds := &dataset.Dataset{
		Peername: book.Username(),
		Name:     name,
		Commit: &dataset.Commit{
			Timestamp: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC),
			Title:     "init dataset",
		},
		Path:         "/ipfs/QmVersion0",
		PreviousPath: "",
	}

	if err = book.WriteVersionSave(ctx, initID, ds, nil); err != nil {
		return ref, err
	}

	ds.Path = "/ipfs/QmVersion1"
	ds.PreviousPath = "/ipfs/QmVesion0"

	if err = book.WriteVersionSave(ctx, initID, ds, nil); err != nil {
		return ref, err
	}

	ds.Path = "/ipfs/QmVersion2"
	ds.PreviousPath = "/ipfs/QmVersion1"

	if err = book.WriteVersionSave(ctx, initID, ds, nil); err != nil {
		return ref, err
	}

	return dsref.Ref{
		Username:  book.Username(),
		Name:      name,
		ProfileID: peerID,
		InitID:    initID,
		Path:      ds.Path,
	}, nil
}
