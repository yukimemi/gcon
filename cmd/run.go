package cmd

func init() {
	funcs["Run"] = F{
		f: (*Gcon).Run,
		p: (*Gcon).ParseRun,
	}
}

// ArgsRun is Run func args.
type ArgsRun struct {
	Target []TaskTarget `require:"true"`
}

// TaskTarget is task info.
type TaskTarget struct {
	ID   string `require:"true"`
	File string `require:"false"`
}

// ParseRun parse args and set ArgsRun.
func (g *Gcon) ParseRun(args Args, ptr interface{}) error {

	if ptr == nil {
		return ParseArgs(args, &ArgsRun{})
	}
	return ParseArgs(args, ptr)
}

// Run output msg.
func (g *Gcon) Run(args Args) (TaskInfo, error) {

	a := ArgsRun{}
	err := g.ParseRun(args, &a)
	if err != nil {
		return TaskInfo{}, err
	}

	for _, target := range a.Target {
		ti := TaskInfo{
			ID:      target.ID,
			Path:    target.File,
			ProType: Normal,
		}
		if ti.Path == "" {
			ti.Path = g.Ti.Path
		}
		err := g.Engine(ti)
		if err != nil {
			return TaskInfo{}, err
		}
	}

	return TaskInfo{}, nil
}
