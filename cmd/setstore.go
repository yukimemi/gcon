package cmd

func init() {
	funcs["SetStore"] = (*Gcon).SetStore
}

// ArgsSet is SetStore func args.
type ArgsSet struct {
	Sets []Set `validate:"required,dive,required"`

	ArgsDef
}

// Set is set key value list.
type Set struct {
	Key string `validate:"required"`
	Val string `validate:"required"`
}

// SetStore set key val to Store.
func (g *Gcon) SetStore(args Args) (*TaskInfo, error) {

	a := &ArgsSet{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	return g.setStore(*a)
}

func (g *Gcon) setStore(a ArgsSet) (*TaskInfo, error) {

	for _, set := range a.Sets {
		g.Set(set.Key, set.Val, true)
	}

	return nil, nil
}
