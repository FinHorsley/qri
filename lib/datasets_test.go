package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/qri-io/dataset"
	"github.com/qri-io/dataset/dsio"
	"github.com/qri-io/dataset/dstest"
	"github.com/qri-io/jsonschema"
	"github.com/qri-io/qfs"
	"github.com/qri-io/qfs/cafs"
	"github.com/qri-io/qri/base"
	"github.com/qri-io/qri/base/dsfs"
	"github.com/qri-io/qri/config"
	"github.com/qri-io/qri/dsref"
	"github.com/qri-io/qri/p2p"
	p2ptest "github.com/qri-io/qri/p2p/test"
	"github.com/qri-io/qri/repo"
	testrepo "github.com/qri-io/qri/repo/test"
)

func TestDatasetRequestsSave(t *testing.T) {
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	jobsBodyPath, err := dstest.BodyFilepath("testdata/jobs_by_automation")
	if err != nil {
		t.Fatal(err.Error())
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := `city,pop,avg_age,in_usa
	toronto,40000000,55.5,false
	new york,8500000,44.4,true
	chicago,300000,44.4,true
	chatham,35000,65.25,true
	raleigh,250000,50.65,true
	sarnia,550000,55.65,false
`
		w.Write([]byte(res))
	}))

	badDataS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`\\\{"json":"data"}`))
	}))

	citiesMetaOnePath := tempDatasetFile(t, "*-cities_meta_1.json", &dataset.Dataset{Meta: &dataset.Meta{Title: "updated name of movies dataset"}})
	citiesMetaTwoPath := tempDatasetFile(t, "*-cities_meta_2.json", &dataset.Dataset{Meta: &dataset.Meta{Description: "Description, b/c bodies are the same thing"}})
	defer func() {
		os.RemoveAll(citiesMetaOnePath)
		os.RemoveAll(citiesMetaTwoPath)
	}()

	req := NewDatasetRequests(node, nil)

	privateErrMsg := "option to make dataset private not yet implimented, refer to https://github.com/qri-io/qri/issues/291 for updates"
	if err := req.Save(&SaveParams{Private: true}, nil); err == nil {
		t.Errorf("expected datset to error")
	} else if err.Error() != privateErrMsg {
		t.Errorf("private flag error mismatch: expected: '%s', got: '%s'", privateErrMsg, err.Error())
	}

	good := []struct {
		description string
		params      SaveParams
		res         *repo.DatasetRef
	}{
		{"body file", SaveParams{Ref: "me/jobs_ranked_by_automation_prob", BodyPath: jobsBodyPath}, nil},
		{"meta set title", SaveParams{Ref: "me/cities", FilePaths: []string{citiesMetaOnePath}}, nil},
		{"meta set description, supply same body", SaveParams{Ref: "me/cities", FilePaths: []string{citiesMetaTwoPath}, BodyPath: s.URL + "/body.csv"}, nil},
	}

	for i, c := range good {
		got := &repo.DatasetRef{}
		err := req.Save(&c.params, got)
		if err != nil {
			t.Errorf("case %d: '%s' unexpected error: %s", i, c.description, err.Error())
			continue
		}

		if got != nil && c.res != nil {
			expect := c.res.Dataset
			gotDs := got.Dataset
			if err := dataset.CompareDatasets(expect, gotDs); err != nil {
				t.Errorf("case %d ds mistmatch: %s", i, err.Error())
				continue
			}
		}
	}

	bad := []struct {
		description string
		params      SaveParams
		err         string
	}{

		{"emtpy params", SaveParams{}, "repo: empty dataset reference"},
		// {&dataset.Dataset{Peername: "foo", Name: "bar"}, nil, "error with previous reference: error fetching peer from store: profile: not found"},
		// {&dataset.Dataset{Peername: "bad", Name: "path", Commit: &dataset.Commit{Qri: "qri:st"}}, nil, "decoding dataset: invalid commit 'qri' value: qri:st"},
		// {&dataset.Dataset{Peername: "bad", Name: "path", BodyPath: "/bad/path"}, nil, "error with previous reference: error fetching peer from store: profile: not found"},
		// {&dataset.Dataset{BodyPath: "testdata/q_bang.svg"}, nil, "invalid data format: unsupported file type: '.svg'"},
		// {&dataset.Dataset{Peername: "me", Name: "cities", BodyPath: "http://localhost:999999/bad/url"}, nil, "fetching body url: Get http://localhost:999999/bad/url: dial tcp: address 999999: invalid port"},
		// {&dataset.Dataset{Name: "bad name", BodyPath: jobsBodyPath}, nil, "invalid name: error: illegal name 'bad name', names must start with a letter and consist of only a-z,0-9, and _. max length 144 characters"},
		// {&dataset.Dataset{BodyPath: jobsBodyPath, Commit: &dataset.Commit{Qri: "qri:st"}}, nil, "decoding dataset: invalid commit 'qri' value: qri:st"},
		{"", SaveParams{Ref: "me/bad", BodyPath: badDataS.URL + "/data.json"}, "determining dataset structure: invalid json data"},
	}

	for i, c := range bad {
		got := &repo.DatasetRef{}
		err := req.Save(&c.params, got)
		if err == nil {
			t.Errorf("case %d: '%s' returned no error", i, c.description)
		}
		if err.Error() != c.err {
			t.Errorf("case %d: '%s' error mismatch. expected:\n'%s'\ngot:\n'%s'", i, c.description, c.err, err.Error())
		}
	}
}

func tempDatasetFile(t *testing.T, fileName string, ds *dataset.Dataset) (path string) {
	f, err := ioutil.TempFile("", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(ds); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestDatasetRequestsForceSave(t *testing.T) {
	node := newTestQriNode(t)
	ref := addCitiesDataset(t, node)
	r := NewDatasetRequests(node, nil)

	res := &repo.DatasetRef{}
	if err := r.Save(&SaveParams{Ref: ref.AliasString()}, res); err == nil {
		t.Error("expected empty save without force flag to error")
	}

	if err := r.Save(&SaveParams{
		Ref:   ref.AliasString(),
		Force: true,
	}, res); err != nil {
		t.Errorf("expected empty save with flag to not error. got: %s", err.Error())
	}
}

func TestDatasetRequestsSaveRecall(t *testing.T) {
	node := newTestQriNode(t)
	ref := addNowTransformDataset(t, node)
	r := NewDatasetRequests(node, nil)

	metaOnePath := tempDatasetFile(t, "*-meta.json", &dataset.Dataset{Meta: &dataset.Meta{Title: "an updated title"}})
	metaTwoPath := tempDatasetFile(t, "*-meta-2.json", &dataset.Dataset{Meta: &dataset.Meta{Title: "new title!"}})
	defer func() {
		os.RemoveAll(metaOnePath)
		os.RemoveAll(metaTwoPath)
	}()

	res := &repo.DatasetRef{}
	err := r.Save(&SaveParams{
		Ref:        ref.AliasString(),
		FilePaths:  []string{metaOnePath},
		ReturnBody: true}, res)
	if err != nil {
		t.Error(err.Error())
	}

	err = r.Save(&SaveParams{
		Ref:       ref.AliasString(),
		FilePaths: []string{metaOnePath},
		Recall:    "wut"}, res)
	if err == nil {
		t.Error("expected bad recall to error")
	}

	err = r.Save(&SaveParams{
		Ref:       ref.AliasString(),
		FilePaths: []string{metaTwoPath},
		Recall:    "tf"}, res)
	if err != nil {
		t.Error(err)
	}
	if res.Dataset.Transform == nil {
		t.Error("expected transform to exist on recalled save")
	}
}

func TestDatasetRequestsSaveZip(t *testing.T) {
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}
	req := NewDatasetRequests(node, nil)

	res := repo.DatasetRef{}
	// TODO (b5): import.zip has a ref.txt file that specifies test_user/test_repo as the dataset name,
	// save now requires a string reference. we need to pick a behaviour here & write a test that enforces it
	err = req.Save(&SaveParams{Ref: "me/huh", FilePaths: []string{"testdata/import.zip"}}, &res)
	if err != nil {
		t.Fatal(err.Error())
	}

	if res.Dataset.Commit.Title != "Test Title" {
		t.Fatalf("Expected 'Test Title', got '%s'", res.Dataset.Commit.Title)
	}
	if res.Dataset.Meta.Title != "Test Repo" {
		t.Fatalf("Expected 'Test Repo', got '%s'", res.Dataset.Meta.Title)
	}
}
func TestDatasetRequestsList(t *testing.T) {
	var (
		movies, counter, cities, craigslist, sitemap repo.DatasetRef
	)

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err)
		return
	}

	refs, err := mr.References(0, 30)
	if err != nil {
		t.Fatalf("error getting namespace: %s", err)
	}

	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err)
	}

	inst := NewInstanceFromConfigAndNode(config.DefaultConfigForTesting(), node)

	for _, ref := range refs {
		switch ref.Name {
		case "movies":
			movies = ref
		case "counter":
			counter = ref
		case "cities":
			cities = ref
		case "craigslist":
			craigslist = ref
		case "sitemap":
			sitemap = ref
		}
	}

	cases := []struct {
		description string
		p           *ListParams
		res         []repo.DatasetRef
		err         string
	}{
		{"list datasets - empty (default)", &ListParams{}, []repo.DatasetRef{cities, counter, craigslist, movies, sitemap}, ""},
		{"list datasets - weird (returns sensible default)", &ListParams{OrderBy: "chaos", Limit: -33, Offset: -50}, []repo.DatasetRef{cities, counter, craigslist, movies, sitemap}, ""},
		{"list datasets - happy path", &ListParams{OrderBy: "", Limit: 30, Offset: 0}, []repo.DatasetRef{cities, counter, craigslist, movies, sitemap}, ""},
		{"list datasets - limit 2 offset 0", &ListParams{OrderBy: "", Limit: 2, Offset: 0}, []repo.DatasetRef{cities, counter}, ""},
		{"list datasets - limit 2 offset 2", &ListParams{OrderBy: "", Limit: 2, Offset: 2}, []repo.DatasetRef{craigslist, movies}, ""},
		{"list datasets - limit 2 offset 4", &ListParams{OrderBy: "", Limit: 2, Offset: 4}, []repo.DatasetRef{sitemap}, ""},
		{"list datasets - limit 2 offset 5", &ListParams{OrderBy: "", Limit: 2, Offset: 5}, []repo.DatasetRef{}, ""},
		{"list datasets - order by timestamp", &ListParams{OrderBy: "timestamp", Limit: 30, Offset: 0}, []repo.DatasetRef{cities, counter, craigslist, movies, sitemap}, ""},
		{"list datasets - peername 'me'", &ListParams{Peername: "me", OrderBy: "timestamp", Limit: 30, Offset: 0}, []repo.DatasetRef{cities, counter, craigslist, movies, sitemap}, ""},
		// TODO: re-enable {&ListParams{OrderBy: "name", Limit: 30, Offset: 0}, []*repo.DatasetRef{cities, counter, movies}, ""},
	}

	req := NewDatasetRequestsInstance(inst)
	for _, c := range cases {
		got := []repo.DatasetRef{}
		err := req.List(c.p, &got)

		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case '%s' error mismatch: expected: %s, got: %s", c.description, c.err, err)
			continue
		}

		if c.err == "" && c.res != nil {
			if len(c.res) != len(got) {
				t.Errorf("case '%s' response length mismatch. expected %d, got: %d", c.description, len(c.res), len(got))
				continue
			}

			for j, expect := range c.res {
				if err := repo.CompareDatasetRef(expect, got[j]); err != nil {
					t.Errorf("case '%s' expected dataset error. index %d mismatch: %s", c.description, j, err.Error())
					continue
				}
			}
		}
	}
}

func TestDatasetRequestsListP2p(t *testing.T) {
	// Matches what is used to generated test peers.
	datasets := []string{"movies", "cities", "counter", "craigslist", "sitemap"}

	ctx := context.Background()
	factory := p2ptest.NewTestNodeFactory(p2p.NewTestableQriNode)
	testPeers, err := p2ptest.NewTestNetwork(ctx, factory, 5)
	if err != nil {
		t.Errorf("error creating network: %s", err.Error())
		return
	}

	if err := p2ptest.ConnectQriNodes(ctx, testPeers); err != nil {
		t.Errorf("error connecting peers: %s", err.Error())
	}

	// Convert from test nodes to non-test nodes.
	peers := make([]*p2p.QriNode, len(testPeers))
	for i, node := range testPeers {
		peers[i] = node.(*p2p.QriNode)
	}

	var wg sync.WaitGroup
	for _, p1 := range peers {
		wg.Add(1)
		go func(node *p2p.QriNode) {
			defer wg.Done()

			dsr := NewDatasetRequests(node, nil)
			p := &ListParams{OrderBy: "", Limit: 30, Offset: 0}
			var res []repo.DatasetRef
			err := dsr.List(p, &res)
			if err != nil {
				t.Errorf("error listing dataset: %s", err.Error())
			}
			// Get number from end of peername, use that to find dataset name.
			profile, _ := node.Repo.Profile()
			num := profile.Peername[len(profile.Peername)-1:]
			index, _ := strconv.ParseInt(num, 10, 32)
			expect := datasets[index]

			if res[0].Name != expect {
				t.Errorf("dataset %s mismatch: %s", res[0].Name, expect)
			}
		}(p1)
	}

	wg.Wait()
}

func TestDatasetRequestsGet(t *testing.T) {
	ctx := context.Background()
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	ref, err := mr.GetRef(repo.DatasetRef{Peername: "peer", Name: "movies"})
	if err != nil {
		t.Fatalf("error getting path: %s", err.Error())
	}

	moviesDs, err := dsfs.LoadDataset(ctx, mr.Store(), ref.Path)
	if err != nil {
		t.Fatalf("error loading dataset: %s", err.Error())
	}

	moviesDs.OpenBodyFile(ctx, node.Repo.Filesystem())
	moviesBodyFile := moviesDs.BodyFile()
	reader := dsio.NewCSVReader(moviesDs.Structure, moviesBodyFile)
	moviesBody := mustBeArray(base.ReadEntries(reader))

	prettyJSONConfig, _ := dataset.NewJSONOptions(map[string]interface{}{"pretty": true})
	nonprettyJSONConfig, _ := dataset.NewJSONOptions(map[string]interface{}{"pretty": false})

	cases := []struct {
		description string
		params      *GetParams
		expect      string
	}{
		{"invalid peer name",
			&GetParams{Path: "peer/ABC@abc"}, "'peer/ABC@abc' is not a valid dataset reference"},

		{"peername with path",
			&GetParams{Path: fmt.Sprintf("peer/ABC@%s", ref.Path)},
			componentToString(setDatasetName(moviesDs, "peer/ABC"), "yaml")},

		{"peername without path",
			&GetParams{Path: "peer/movies"},
			componentToString(setDatasetName(moviesDs, "peer/movies"), "yaml")},

		{"peername as json format",
			&GetParams{Path: "peer/movies", Format: "json"},
			componentToString(setDatasetName(moviesDs, "peer/movies"), "json")},

		{"commit component",
			&GetParams{Path: "peer/movies", Selector: "commit"},
			componentToString(moviesDs.Commit, "yaml")},

		{"commit component as json format",
			&GetParams{Path: "peer/movies", Selector: "commit", Format: "json"},
			componentToString(moviesDs.Commit, "json")},

		{"title field of commit component",
			&GetParams{Path: "peer/movies", Selector: "commit.title"}, "initial commit\n"},

		{"title field of commit component as json",
			&GetParams{Path: "peer/movies", Selector: "commit.title", Format: "json"},
			"\"initial commit\""},

		{"title field of commit component as yaml",
			&GetParams{Path: "peer/movies", Selector: "commit.title", Format: "yaml"},
			"initial commit\n"},

		{"title field of commit component as mispelled format",
			&GetParams{Path: "peer/movies", Selector: "commit.title", Format: "jason"},
			"unknown format: \"jason\""},

		{"body as json",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json"}, "[]"},

		{"dataset empty",
			&GetParams{Path: "", Selector: "body", Format: "json"}, "repo: empty dataset reference"},

		{"body as csv",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "csv"}, "title,duration\n"},

		{"body with limit and offfset",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json",
				Limit: 5, Offset: 0, All: false}, bodyToString(moviesBody[:5])},

		{"body with invalid limit and offset",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json",
				Limit: -5, Offset: -100, All: false}, "invalid limit / offset settings"},

		{"body with all flag ignores invalid limit and offset",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json",
				Limit: -5, Offset: -100, All: true}, bodyToString(moviesBody)},

		{"body with all flag",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json",
				Limit: 0, Offset: 0, All: true}, bodyToString(moviesBody)},

		{"body with limit and non-zero offset",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json",
				Limit: 2, Offset: 10, All: false}, bodyToString(moviesBody[10:12])},

		{"head non-pretty json",
			&GetParams{Path: "peer/movies", Format: "json", FormatConfig: nonprettyJSONConfig},
			componentToString(setDatasetName(moviesDs, "peer/movies"), "non-pretty json")},

		{"body pretty json",
			&GetParams{Path: "peer/movies", Selector: "body", Format: "json",
				FormatConfig: prettyJSONConfig, Limit: 3, Offset: 0, All: false},
			bodyToPrettyString(moviesBody[:3])},
	}

	req := NewDatasetRequests(node, nil)
	for _, c := range cases {
		got := &GetResult{}
		err := req.Get(c.params, got)
		if err != nil {
			if err.Error() != c.expect {
				t.Errorf("case \"%s\": error mismatch: expected: %s, got: %s", c.description, c.expect, err)
			}
			continue
		}

		result := string(got.Bytes)
		if result != c.expect {
			t.Errorf("case \"%s\": failed, expected:\n\"%s\", got:\n\"%s\"", c.description, c.expect, result)
		}
	}
}

func setDatasetName(ds *dataset.Dataset, name string) *dataset.Dataset {
	parts := strings.Split(name, "/")
	ds.Peername = parts[0]
	ds.Name = parts[1]
	return ds
}

func componentToString(component interface{}, format string) string {
	switch format {
	case "json":
		bytes, err := json.MarshalIndent(component, "", " ")
		if err != nil {
			return err.Error()
		}
		return string(bytes)
	case "non-pretty json":
		bytes, err := json.Marshal(component)
		if err != nil {
			return err.Error()
		}
		return string(bytes)
	case "yaml":
		bytes, err := yaml.Marshal(component)
		if err != nil {
			return err.Error()
		}
		return string(bytes)
	default:
		return "Unknown format"
	}
}

func bodyToString(component interface{}) string {
	bytes, err := json.Marshal(component)
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

func bodyToPrettyString(component interface{}) string {
	bytes, err := json.MarshalIndent(component, "", " ")
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

func TestDatasetRequestsGetP2p(t *testing.T) {
	// Matches what is used to generated test peers.
	datasets := []string{"movies", "cities", "counter", "craigslist", "sitemap"}

	ctx := context.Background()
	factory := p2ptest.NewTestNodeFactory(p2p.NewTestableQriNode)
	testPeers, err := p2ptest.NewTestNetwork(ctx, factory, 5)
	if err != nil {
		t.Errorf("error creating network: %s", err.Error())
		return
	}

	if err := p2ptest.ConnectQriNodes(ctx, testPeers); err != nil {
		t.Errorf("error connecting peers: %s", err.Error())
	}

	// Convert from test nodes to non-test nodes.
	peers := make([]*p2p.QriNode, len(testPeers))
	for i, node := range testPeers {
		peers[i] = node.(*p2p.QriNode)
	}

	var wg sync.WaitGroup
	for _, p1 := range peers {
		wg.Add(1)
		go func(node *p2p.QriNode) {
			defer wg.Done()
			// Get number from end of peername, use that to create dataset name.
			profile, _ := node.Repo.Profile()
			num := profile.Peername[len(profile.Peername)-1:]
			index, _ := strconv.ParseInt(num, 10, 32)
			name := datasets[index]
			ref := repo.DatasetRef{Peername: profile.Peername, Name: name}

			dsr := NewDatasetRequests(node, nil)
			got := &GetResult{}
			err = dsr.Get(&GetParams{Path: ref.String()}, got)
			if err != nil {
				t.Errorf("error listing dataset for %s: %s", ref.Name, err.Error())
			}

			if got.Bytes == nil {
				t.Errorf("failed to get dataset for %s", ref.Name)
			}
			// TODO: Test contents of Dataset.
		}(p1)
	}

	wg.Wait()
}

func TestDatasetRequestsRename(t *testing.T) {
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	bad := []struct {
		p   *RenameParams
		err string
	}{
		{&RenameParams{}, "current name is required to rename a dataset"},
		{&RenameParams{Current: repo.DatasetRef{Peername: "peer", Name: "movies"}, New: repo.DatasetRef{Peername: "peer", Name: "new movies"}}, "error: illegal name 'new movies', names must start with a letter and consist of only a-z,0-9, and _. max length 144 characters"},
		{&RenameParams{Current: repo.DatasetRef{Peername: "peer", Name: "cities"}, New: repo.DatasetRef{Peername: "peer", Name: "sitemap"}}, "dataset 'peer/sitemap' already exists"},
	}

	req := NewDatasetRequests(node, nil)
	for i, c := range bad {
		got := &repo.DatasetRef{}
		err := req.Rename(c.p, got)

		if err == nil {
			t.Errorf("case %d didn't error. expected: %s", i, c.err)
			continue
		}

		if c.err != err.Error() {
			t.Errorf("case %d error mismatch: expected: %s, got: %s", i, c.err, err)
			continue
		}
	}

	log, err := mr.Logbook().DatasetRef(dsref.Ref{Username: "peer", Name: "movies"})
	if err != nil {
		t.Errorf("error getting logbook head reference: %s", err)
	}

	p := &RenameParams{
		Current: repo.DatasetRef{Peername: "peer", Name: "movies"},
		New:     repo.DatasetRef{Peername: "peer", Name: "new_movies"},
	}

	res := &repo.DatasetRef{}
	if err := req.Rename(p, res); err != nil {
		t.Errorf("unexpected error renaming: %s", err)
	}

	expect := &repo.DatasetRef{Peername: "peer", Name: "new_movies"}
	if expect.AliasString() != res.AliasString() {
		t.Errorf("response mismatch. expected: %s, got: %s", expect.AliasString(), res.AliasString())
	}

	// get log by id this time
	after, err := mr.Logbook().Log(log.ID())
	if err != nil {
		t.Errorf("getting log by ID: %s", err)
	}

	if expect.Name != after.Name() {
		t.Errorf("rename log mismatch. expected: %s, got: %s", expect.Name, after.Name())
	}
}

func TestDatasetRequestsRemove(t *testing.T) {
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	inst := NewInstanceFromConfigAndNode(config.DefaultConfigForTesting(), node)

	// TODO (b5) - dataset requests require an instance to delete properly
	// we should do the "DatasetMethods" refactor ASAP
	req := NewDatasetRequestsInstance(inst)
	allRevs := dsref.Rev{Field: "ds", Gen: -1}

	// we need some fsi stuff to fully test remove
	fsim := NewFSIMethods(inst)
	// create datasets working directory
	datasetsDir, err := ioutil.TempDir("", "QriTestDatasetRequestsRemove")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(datasetsDir)

	// initialize an example no-history dataset
	initp := &InitFSIDatasetParams{
		Name:   "no_history",
		Dir:    datasetsDir,
		Format: "csv",
		Mkdir:  "no_history",
	}
	var noHistoryName string
	if err := fsim.InitDataset(initp, &noHistoryName); err != nil {
		t.Fatal(err)
	}

	// link cities dataset with a checkout
	checkoutp := &CheckoutParams{
		Dir: filepath.Join(datasetsDir, "cities"),
		Ref: "me/cities",
	}
	var out string
	if err := fsim.Checkout(checkoutp, &out); err != nil {
		t.Fatal(err)
	}

	// add a commit to craigslist
	saveRes := &repo.DatasetRef{}
	if err := req.Save(&SaveParams{Ref: "peer/craigslist", Dataset: &dataset.Dataset{Meta: &dataset.Meta{Title: "oh word"}}}, saveRes); err != nil {
		t.Fatal(err)
	}

	// link craigslist with a checkout
	checkoutp = &CheckoutParams{
		Dir: filepath.Join(datasetsDir, "craigslist"),
		Ref: "me/craigslist",
	}
	if err := fsim.Checkout(checkoutp, &out); err != nil {
		t.Fatal(err)
	}

	badCases := []struct {
		err    string
		params RemoveParams
	}{
		{"repo: empty dataset reference", RemoveParams{Ref: "", Revision: allRevs}},
		{"repo: not found", RemoveParams{Ref: "abc/ABC", Revision: allRevs}},
		{"can only remove whole dataset versions, not individual components", RemoveParams{Ref: "abc/ABC", Revision: dsref.Rev{Field: "st", Gen: -1}}},
		{"invalid number of revisions to delete: 0", RemoveParams{Ref: "peer/movies", Revision: dsref.Rev{Field: "ds", Gen: 0}}},
		{"cannot unlink, dataset is not linked to a directory", RemoveParams{Ref: "peer/movies", Revision: allRevs, Unlink: true}},
		{"can't delete files, dataset is not linked to a directory", RemoveParams{Ref: "peer/movies", Revision: allRevs, DeleteFSIFiles: true}},
	}

	for _, c := range badCases {
		t.Run(fmt.Sprintf("bad_case_%s", c.err), func(t *testing.T) {
			res := RemoveResponse{}
			err := req.Remove(&c.params, &res)

			if err == nil {
				t.Errorf("expected error. got nil")
				return
			} else if c.err != err.Error() {
				t.Errorf("error mismatch: expected: %s, got: %s", c.err, err)
			}
		})
	}

	goodCases := []struct {
		description string
		params      RemoveParams
		res         RemoveResponse
	}{
		{"all generations of peer/movies",
			RemoveParams{Ref: "peer/movies", Revision: allRevs},
			RemoveResponse{NumDeleted: -1},
		},
		{"all generations, specifying more revs than log length",
			RemoveParams{Ref: "peer/counter", Revision: dsref.Rev{Field: "ds", Gen: 20}},
			RemoveResponse{NumDeleted: -1},
		},
		{"all generations of peer/cities, remove link, delete files",
			RemoveParams{Ref: "peer/cities", Revision: allRevs, DeleteFSIFiles: true},
			RemoveResponse{NumDeleted: -1, Unlinked: true, DeletedFSIFiles: true},
		},
		{"one commit of peer/craigslist, remove link, delete files",
			RemoveParams{Ref: "peer/craigslist", Revision: dsref.Rev{Field: "ds", Gen: 1}, Unlink: true, DeleteFSIFiles: true},
			RemoveResponse{NumDeleted: 1, Unlinked: true, DeletedFSIFiles: true},
		},
		{"no history dataset, remove link, delete files",
			RemoveParams{Ref: noHistoryName, Revision: dsref.Rev{Field: "ds", Gen: 0}, DeleteFSIFiles: true},
			RemoveResponse{NumDeleted: 0, Unlinked: true, DeletedFSIFiles: true},
		},
	}

	for _, c := range goodCases {
		t.Run(fmt.Sprintf("good_case_%s", c.description), func(t *testing.T) {
			res := RemoveResponse{}
			err := req.Remove(&c.params, &res)

			if err != nil {
				t.Errorf("unexpected error: %s", err)
				return
			}
			if c.res.NumDeleted != res.NumDeleted {
				t.Errorf("res.NumDeleted mismatch. want %d, got %d", c.res.NumDeleted, res.NumDeleted)
			}
			if c.res.Unlinked != res.Unlinked {
				t.Errorf("res.Unlinked mismatch. want %t, got %t", c.res.Unlinked, res.Unlinked)
			}
			if c.res.DeletedFSIFiles != res.DeletedFSIFiles {
				t.Errorf("res.DeletedFSIFiles mismatch. want %t, got %t", c.res.DeletedFSIFiles, res.DeletedFSIFiles)
			}
		})
	}
}

func TestDatasetRequestsAdd(t *testing.T) {
	t.Skip("TODO (b5)")
	cases := []struct {
		p   *AddParams
		res *repo.DatasetRef
		err string
	}{
		{&AddParams{Ref: "abc/hash###"}, nil, "node is not online and no registry is configured"},
	}

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	req := NewDatasetRequests(node, nil)
	for i, c := range cases {
		got := &repo.DatasetRef{}
		err := req.Add(c.p, got)

		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case %d error mismatch: expected: %s, got: %s", i, c.err, err)
			continue
		}
	}
}

func TestDatasetRequestsAddP2P(t *testing.T) {
	t.Skip("TODO (b5)")
	// Matches what is used to generate the test peers.
	datasets := []string{"movies", "cities", "counter", "craigslist", "sitemap"}

	// Create test nodes.
	ctx := context.Background()
	factory := p2ptest.NewTestNodeFactory(p2p.NewTestableQriNode)
	testPeers, err := p2ptest.NewTestNetwork(ctx, factory, 5)
	if err != nil {
		t.Errorf("error creating network: %s", err.Error())
		return
	}

	// Peers exchange Qri profile information.
	if err := p2ptest.ConnectQriNodes(ctx, testPeers); err != nil {
		t.Errorf("error upgrading to qri connections: %s", err.Error())
		return
	}

	// Convert from test nodes to non-test nodes.
	peers := make([]*p2p.QriNode, len(testPeers))
	for i, node := range testPeers {
		peers[i] = node.(*p2p.QriNode)
	}

	// Connect in memory Mapstore's behind the scene to simulate IPFS like behavior.
	for i, s0 := range peers {
		for _, s1 := range peers[i+1:] {
			m0 := (s0.Repo.Store()).(*cafs.MapStore)
			m1 := (s1.Repo.Store()).(*cafs.MapStore)
			m0.AddConnection(m1)
		}
	}

	var wg sync.WaitGroup
	for i, p0 := range peers {
		for _, p1 := range peers[i+1:] {
			wg.Add(1)
			go func(p0, p1 *p2p.QriNode) {
				defer wg.Done()

				// Get ref to dataset that peer2 has.
				profile, _ := p1.Repo.Profile()
				num := profile.Peername[len(profile.Peername)-1:]
				index, _ := strconv.ParseInt(num, 10, 32)
				name := datasets[index]
				ref := repo.DatasetRef{Peername: profile.Peername, Name: name}
				p := &AddParams{
					Ref: ref.AliasString(),
				}

				// Build requests for peer1 to peer2.
				dsr := NewDatasetRequests(p0, nil)
				got := &repo.DatasetRef{}

				err := dsr.Add(p, got)
				if err != nil {
					pro1, _ := p0.Repo.Profile()
					pro2, _ := p1.Repo.Profile()
					t.Errorf("error adding dataset for %s from %s to %s: %s",
						ref.Name, pro2.Peername, pro1.Peername, err.Error())
				}
			}(p0, p1)
		}
	}
	wg.Wait()

	// TODO: Validate that p1 has added data from p2.
}

func TestDatasetRequestsValidate(t *testing.T) {
	movieb := []byte(`movie_title,duration
Avatar ,178
Pirates of the Caribbean: At World's End ,169
Pirates of the Caribbean: At World's End ,foo
`)
	schemaB := []byte(`{
	  "type": "array",
	  "items": {
	    "type": "array",
	    "items": [
	      {
	        "title": "title",
	        "type": "string"
	      },
	      {
	        "title": "duration",
	        "type": "number"
	      }
	    ]
	  }
	}`)

	dataf := qfs.NewMemfileBytes("data.csv", movieb)
	dataf2 := qfs.NewMemfileBytes("data.csv", movieb)
	schemaf := qfs.NewMemfileBytes("schema.json", schemaB)
	schemaf2 := qfs.NewMemfileBytes("schema.json", schemaB)

	cases := []struct {
		p         ValidateDatasetParams
		numErrors int
		err       string
	}{
		{ValidateDatasetParams{Ref: ""}, 0, "bad arguments provided"},
		{ValidateDatasetParams{Ref: "me"}, 0, "cannot find dataset: peer"},
		{ValidateDatasetParams{Ref: "me/movies"}, 4, ""},
		{ValidateDatasetParams{Ref: "me/movies", Body: dataf, BodyFilename: "data.csv"}, 1, ""},
		{ValidateDatasetParams{Ref: "me/movies", Schema: schemaf}, 4, ""},
		{ValidateDatasetParams{Schema: schemaf2, BodyFilename: "data.csv", Body: dataf2}, 1, ""},
	}

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	req := NewDatasetRequests(node, nil)
	for i, c := range cases {
		got := []jsonschema.ValError{}
		err := req.Validate(&c.p, &got)
		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case %d error mismatch: expected: %s, got: %s", i, c.err, err.Error())
			continue
		}

		if len(got) != c.numErrors {
			t.Errorf("case %d error count mismatch. expected: %d, got: %d", i, c.numErrors, len(got))
			t.Log(got)
			continue
		}
	}
}

func TestDatasetRequestsStats(t *testing.T) {
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Fatalf("error allocating test repo: %s", err.Error())
	}
	node, err := p2p.NewQriNode(mr, config.DefaultP2PForTesting())
	if err != nil {
		t.Fatal(err.Error())
	}

	inst := NewInstanceFromConfigAndNode(config.DefaultConfigForTesting(), node)
	req := NewDatasetRequestsInstance(inst)

	badCases := []struct {
		description string
		ref         string
		expectedErr string
	}{
		{"empty reference", "", repo.ErrEmptyRef.Error()},
		{"dataset does not exist", "me/dataset_does_not_exist", "repo: not found"},
	}
	for i, c := range badCases {
		res := &StatsResponse{}
		err := req.Stats(&StatsParams{Ref: c.ref}, res)
		if c.expectedErr != err.Error() {
			t.Errorf("%d. case %s: error mismatch, expected: '%s', got: '%s'", i, c.description, c.expectedErr, err.Error())
		}
	}

	// TODO (ramfox): see if there is a better way to verify the stat bytes then
	// just inputing them in the cases struct
	goodCases := []struct {
		description string
		ref         string
		expected    []byte
	}{
		{"csv: me/cities", "me/cities", []byte(`[{"count":5,"maxLength":8,"minLength":7,"type":"string","unique":5},{"count":5,"max":40000000,"min":35000,"type":"numeric","unique":5},{"count":5,"frequencies":{"44.4":2},"max":65.25,"min":44.4,"type":"numeric","unique":3},{"count":5,"falseCount":1,"trueCount":4,"type":"boolean"}]`)},
		{"json: me/sitemap", "me/sitemap", []byte(`[{"count":10,"key":"contentLength","max":40079,"min":24515,"type":"numeric","unique":10},{"count":10,"frequencies":{"text/html; charset=utf-8":10},"key":"contentSniff","maxLength":24,"minLength":24,"type":"string"},{"count":10,"frequencies":{"text/html; charset=utf-8":10},"key":"contentType","maxLength":24,"minLength":24,"type":"string"},{"count":10,"key":"duration","max":4081577841,"min":74291866,"type":"numeric","unique":10},{"count":10,"key":"hash","maxLength":68,"minLength":68,"type":"string","unique":10},{"key":"links","type":"array","values":[{"count":10,"maxLength":58,"minLength":14,"unique":10},{"count":10,"maxLength":115,"minLength":19,"unique":10},{"count":10,"maxLength":68,"minLength":22,"unique":10},{"count":10,"maxLength":115,"minLength":14,"unique":10},{"count":9,"maxLength":70,"minLength":15,"unique":9},{"count":9,"maxLength":115,"minLength":37,"unique":9},{"count":9,"maxLength":52,"minLength":15,"unique":9},{"count":9,"maxLength":75,"minLength":19,"unique":9},{"count":9,"maxLength":66,"minLength":15,"unique":9},{"count":7,"maxLength":75,"minLength":19,"unique":7},{"count":7,"maxLength":66,"minLength":22,"unique":7},{"count":6,"maxLength":43,"minLength":19,"unique":6},{"count":6,"maxLength":77,"minLength":14,"unique":6},{"count":6,"maxLength":77,"minLength":21,"unique":6},{"count":4,"maxLength":43,"minLength":14,"unique":4},{"count":3,"maxLength":32,"minLength":21,"unique":3},{"count":3,"maxLength":42,"minLength":19,"unique":3},{"count":3,"maxLength":66,"minLength":32,"unique":3},{"count":3,"maxLength":46,"minLength":19,"unique":3},{"count":2,"maxLength":66,"minLength":22,"unique":2},{"count":2,"maxLength":32,"minLength":23,"unique":2},{"count":2,"maxLength":33,"minLength":22,"unique":2},{"count":2,"maxLength":32,"minLength":27,"unique":2},{"count":1,"maxLength":33,"minLength":33,"unique":1},{"count":1,"maxLength":27,"minLength":27,"unique":1}]},{"count":1,"key":"redirectTo","maxLength":18,"minLength":18,"type":"string","unique":1},{"count":11,"frequencies":{"200":10},"key":"status","max":301,"min":200,"type":"numeric","unique":1},{"count":11,"key":"timestamp","maxLength":35,"minLength":35,"type":"string","unique":11},{"count":10,"key":"title","maxLength":88,"minLength":53,"type":"string","unique":10},{"count":11,"key":"url","maxLength":78,"minLength":18,"type":"string","unique":11}]`)},
	}
	for i, c := range goodCases {
		res := &StatsResponse{}
		err := req.Stats(&StatsParams{Ref: c.ref}, res)
		if err != nil {
			t.Errorf("%d. case %s: unexpected error: '%s'", i, c.description, err.Error())
			continue
		}
		got, err := ioutil.ReadAll(res.Reader)
		if err != nil {
			t.Fatalf("%d. case %s: error reading response: '%s'", i, c.description, err.Error())
		}
		if diff := cmp.Diff(c.expected, got); diff != "" {
			t.Errorf("%d. '%s' result mismatch (-want +got):%s\n", i, c.description, diff)
		}
	}
}

// Convert the interface value into an array, or panic if not possible
func mustBeArray(i interface{}, err error) []interface{} {
	if err != nil {
		panic(err)
	}
	return i.([]interface{})
}
