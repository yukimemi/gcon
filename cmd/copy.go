package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yukimemi/file"
)

func init() {
	funcs["Copy"] = (*Gcon).Copy
}

// ArgsCopy is Copy func args.
type ArgsCopy struct {
	Pairs []Pair `validate:"required,dive,required"`
}

// Pair is copy src dst pair.
type Pair struct {
	Src     string `validate:"required"`
	Dst     string `validate:"required"`
	Recurse bool
	Filter  Filter
	Force   bool
}

// Filter is file and directory filter.
type Filter struct {
	Matches []string
	Ignores []string
}

// Copy copy file or directory src to dst.
func (g *Gcon) Copy(args Args) (TaskInfo, error) {

	a := &ArgsCopy{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return TaskInfo{}, err
	}

	for _, pair := range a.Pairs {
		infos, err := file.GetInfos(pair.Src, file.Option{
			Matches: pair.Filter.Matches,
			Ignores: pair.Filter.Ignores,
			Recurse: pair.Recurse,
		})
		if err != nil {
			return TaskInfo{}, err
		}
		for info := range infos {
			if info.Err != nil {
				return TaskInfo{}, info.Err
			}

			dstPath := strings.Replace(info.Path, pair.Src, pair.Dst, 1)

			if info.Fi.IsDir() {
				g.Debugf("Mkdir: [%v]", dstPath)
				err := os.MkdirAll(dstPath, os.ModePerm)
				if err != nil {
					return TaskInfo{}, err
				}
			} else {
				os.MkdirAll(filepath.Dir(dstPath), os.ModePerm)
				g.Debugf("[%v] -> [%v] (before)", info.Path, dstPath)
				n, err := file.Copy(info.Path, dstPath, pair.Force)
				if err != nil {
					return TaskInfo{}, err
				}
				g.Infof("[%v] -> [%v] (%v bytes)", info.Path, dstPath, n)
			}
		}
	}

	return TaskInfo{}, nil
}
