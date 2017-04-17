package cmd

import "fmt"

func init() {
	funcs["Log"] = F{
		f: (*Gcon).Log,
		p: (*Gcon).ParseLog,
	}
}

// ArgsLog is Log func args.
type ArgsLog struct {
	Msg    string `require:"true"`
	Stdout bool   `require:"false"`
}

// ParseLog parse args and set ArgsLog.
func (g *Gcon) ParseLog(args Args, ptr interface{}) error {

	if ptr == nil {
		return ParseArgs(args, &ArgsLog{})
	}
	return ParseArgs(args, ptr)
}

// Log output msg.
func (g *Gcon) Log(args Args) (TaskInfo, error) {

	a := ArgsLog{}
	err := g.ParseLog(args, &a)
	if err != nil {
		return TaskInfo{}, err
	}
	if a.Stdout {
		fmt.Println(a.Msg)
	}

	return TaskInfo{}, nil
}
