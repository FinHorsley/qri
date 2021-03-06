package dsfs

import (
	"context"
	"fmt"

	"github.com/qri-io/dataset"
	"github.com/qri-io/qfs"
)

// DerefReadme dereferences a dataset's Readme element if required
// no-op if ds.Readme is nil or isn't a reference
func DerefReadme(ctx context.Context, store qfs.Filesystem, ds *dataset.Dataset) error {
	if ds.Readme != nil && ds.Readme.IsEmpty() && ds.Readme.Path != "" {
		rm, err := loadReadme(ctx, store, ds.Readme.Path)
		if err != nil {
			log.Debug(err.Error())
			return fmt.Errorf("loading dataset readme: %w", err)
		}
		rm.Path = ds.Readme.Path
		ds.Readme = rm
	}
	return nil
}

// loadReadme assumes the provided path is valid
func loadReadme(ctx context.Context, fs qfs.Filesystem, path string) (st *dataset.Readme, err error) {
	data, err := fileBytes(fs.Get(ctx, path))
	if err != nil {
		log.Debug(err.Error())
		return nil, fmt.Errorf("error loading readme file: %w", err)
	}
	return dataset.UnmarshalReadme(data)
}

// LoadReadmeScript loads script data from a dataset path if the given dataset has a readme script is specified
// the returned qfs.File will be the value of dataset.Readme.ScriptPath
func LoadReadmeScript(ctx context.Context, fs qfs.Filesystem, dspath string) (qfs.File, error) {
	ds, err := LoadDataset(ctx, fs, dspath)
	if err != nil {
		return nil, err
	}

	if ds.Readme == nil || ds.Readme.ScriptPath == "" {
		return nil, ErrNoReadme
	}

	return fs.Get(ctx, ds.Readme.ScriptPath)
}

func addReadmeFile(ds *dataset.Dataset, wfs *writeFiles) error {
	if ds.Readme == nil {
		return nil
	}

	ds.Readme.DropTransientValues()
	if rmsf := ds.Readme.ScriptFile(); rmsf != nil {
		wfs.readmeScript = qfs.NewMemfileReader(PackageFileReadmeScript.Filename(), rmsf)
	}
	return nil
}
