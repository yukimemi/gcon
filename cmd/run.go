package cmd

func init() {
	funcs["Run"] = (*Gcon).Run
}

// ArgsRun is Run func args.
type ArgsRun struct {
	Targets []Target `validate:"required,dive,required"`

	ArgsDef
}

// Target is task info.
type Target struct {
	ID   string `validate:"required"`
	File string
}

// Run output msg.
func (g *Gcon) Run(args Args) (*TaskInfo, error) {

	a := &ArgsRun{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	for _, target := range a.Targets {
		ti := &TaskInfo{
			ID:      target.ID,
			Path:    target.File,
			ProType: Normal,
		}
		if ti.Path == "" {
			ti.Path = g.Ti.Path
		}
		err := g.Engine(ti)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}
