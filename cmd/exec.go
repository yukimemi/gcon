package cmd

import (
	"strings"

	"golang.org/x/text/encoding/japanese"

	"github.com/yukimemi/core"
)

func init() {
	funcs["Exec"] = (*Gcon).Exec
}

// ArgsExec is Exec func args.
type ArgsExec struct {
	Cmds []Cmd `validate:"required,dive,required"`

	ArgsDef
}

// Cmd is command exec list.
type Cmd struct {
	Cmd     string `validate:"required"`
	Arg     string
	IP      string
	Sjis    bool
	Results []Result
	Other   Target
}

// Result is exit code pattern task.
type Result struct {
	Code int
	Target
}

// Parse parse ArgsExec.
func (a *ArgsExec) Parse(args Args) {
	return ParseArg(args, ad)
}

// Exec execute process.
func (g *Gcon) Exec(args Args) (*TaskInfo, error) {

	a := &ArgsExec{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	// Execute cmd list.
	for _, cmd := range a.Cmds {
		c := core.Cmd{CmdLine: cmd.Cmd + " " + cmd.Arg}
		if cmd.Sjis {
			c.StdoutEnc = japanese.ShiftJIS.NewDecoder()
		}
		soScanner, err := c.StdoutScanner()
		if err != nil {
			return nil, err
		}
		seScanner, err := c.StderrScanner()
		if err != nil {
			return nil, err
		}

		err = c.CmdStart()
		if err != nil {
			return nil, err
		}
		// Asynchronous output log.
		c.Wg.Add(1)
		go func() {
			defer c.Wg.Done()
			for soScanner.Scan() {
				g.Infof(soScanner.Text())
			}
		}()
		c.Wg.Add(1)
		go func() {
			defer c.Wg.Done()
			for seScanner.Scan() {
				g.Errorf(seScanner.Text())
			}
		}()

		c.CmdWait()

		g.Infof("Cmd:      [%v]", c.Cmd.Args[0])
		g.Infof("Arg:      [%v]", strings.Join(c.Cmd.Args[1:], " "))
		g.Infof("ExitCode: [%v]", c.ExitCode)
		g.Infof("Stdout:   [%v]", c.Stdout.String())
		g.Infof("Stderr:   [%v]", c.Stderr.String())

		target := new(Target)
		for _, result := range cmd.Results {
			if c.ExitCode == result.Code && result.ID != "" {
				*target = result.Target
				break
			}
		}

		if target.ID == "" && cmd.Other.ID != "" {
			*target = cmd.Other
		}

		// Run result or other target.
		ar := ArgsRun{Targets: []Target{*target}}
		ti, err := g.Run(ar)
		if err != nil {
			return ti, err
		}

	}

	// Get target async func.
	return nil, nil
}
