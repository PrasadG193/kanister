package envdir

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func EnvDir(dir string) ([]string, error) {
	// Check if dir doesn't exist or it isn't a directory.
	if fi, err := os.Stat(dir); os.IsNotExist(err) || !fi.IsDir() {
		return nil, nil
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read env from dir:"+dir)
	}
	e := make([]string, 0, len(fis))
	for _, fi := range fis {
		if fi.IsDir() || fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}
		p := filepath.Join(dir, fi.Name())
		f, err := os.Open(p)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read env from dir:"+dir)
		}
		c, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read env from dir:"+dir)
		}
		e = append(e, fmt.Sprintf("%s=%s", fi.Name(), c))
	}
	return e, nil
}
