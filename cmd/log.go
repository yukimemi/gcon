package cmd

import "fmt"

func init() {
	funcs["Log"] = (*Gcon).Log
}

// ArgsLog is Log func args.
type ArgsLog struct {
	Msg    string `require:"true"`
	Stdout bool   `require:"false"`
}

// Log output msg.
func (g *Gcon) Log(args Args) (TaskInfo, error) {

	a := &ArgsLog{}
	err := ParseArgs(args, a)
	if err != nil {
		return TaskInfo{}, err
	}
	if a.Stdout {
		fmt.Println(a.Msg)
	}

	return TaskInfo{}, nil
}
