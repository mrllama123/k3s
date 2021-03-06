package tmpfssymlink

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/rootless-containers/rootlesskit/pkg/common"
	"github.com/rootless-containers/rootlesskit/pkg/copyup"
)

func NewChildDriver() copyup.ChildDriver {
	return &childDriver{}
}

type childDriver struct {
}

func (d *childDriver) CopyUp(dirs []string) ([]string, error) {
	// we create bind0 outside of StateDir so as to allow
	// copying up /run with stateDir=/run/user/1001/rootlesskit/default.
	bind0, err := ioutil.TempDir("/tmp", "rootlesskit-b")
	if err != nil {
		return nil, errors.Wrap(err, "creating bind0 directory under /tmp")
	}
	defer os.RemoveAll(bind0)
	var copied []string
	for _, d := range dirs {
		d := filepath.Clean(d)
		if d == "/tmp" {
			// TODO: we can support copy-up /tmp by changing bind0TempDir
			return copied, errors.New("/tmp cannot be copied up")
		}
		cmds := [][]string{
			// TODO: read-only bind (does not work well for /run)
			{"mount", "--rbind", d, bind0},
			{"mount", "-n", "-t", "tmpfs", "none", d},
		}
		if err := common.Execs(os.Stderr, os.Environ(), cmds); err != nil {
			return copied, errors.Wrapf(err, "executing %v", cmds)
		}
		bind1, err := ioutil.TempDir(d, ".ro")
		if err != nil {
			return copied, errors.Wrapf(err, "creating a directory under %s", d)
		}
		cmds = [][]string{
			{"mount", "-n", "--move", bind0, bind1},
		}
		if err := common.Execs(os.Stderr, os.Environ(), cmds); err != nil {
			return copied, errors.Wrapf(err, "executing %v", cmds)
		}
		files, err := ioutil.ReadDir(bind1)
		if err != nil {
			return copied, errors.Wrapf(err, "reading dir %s", bind1)
		}
		for _, f := range files {
			fFull := filepath.Join(bind1, f.Name())
			var symlinkSrc string
			if f.Mode()&os.ModeSymlink != 0 {
				symlinkSrc, err = os.Readlink(fFull)
				if err != nil {
					return copied, errors.Wrapf(err, "reading dir %s", fFull)
				}
			} else {
				symlinkSrc = filepath.Join(filepath.Base(bind1), f.Name())
			}
			symlinkDst := filepath.Join(d, f.Name())
			// `mount` may create extra `/etc/mtab` after mounting empty tmpfs on /etc
			// https://github.com/rootless-containers/rootlesskit/issues/45
			if err = os.RemoveAll(symlinkDst); err != nil {
				return copied, errors.Wrapf(err, "removing %s", symlinkDst)
			}
			if err := os.Symlink(symlinkSrc, symlinkDst); err != nil {
				return copied, errors.Wrapf(err, "symlinking %s to %s", symlinkSrc, symlinkDst)
			}
		}
		copied = append(copied, d)
	}
	return copied, nil
}
