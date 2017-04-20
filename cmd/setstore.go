package cmd

func init() {
	funcs["SetStore"] = (*Gcon).SetStore
}

// ArgsSet is SetStore func args.
type ArgsSet struct {
	Sets []Set `require:"true"`
}

// Set is set key value list.
type Set struct {
	Key string `require:"true"`
	Val string `require:"true"`
}

// SetStore set key val to Store.
func (g *Gcon) SetStore(args Args) (TaskInfo, error) {

	a := &ArgsSet{}
	err := ParseArgs(args, a)
	if err != nil {
		return TaskInfo{}, err
	}

	for _, set := range a.Sets {
		g.Set(set.Key, set.Val, true)
	}

	return TaskInfo{}, nil
}
