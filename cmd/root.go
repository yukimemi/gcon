// Copyright © 2017 yukimemi <yukimemi@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yukimemi/file"
	"gopkg.in/yaml.v2"
)

const (
	// ExitOK is success exit code.
	ExitOK = iota
	// ExitNG is error exit code.
	ExitNG
)

const (
	// Normal is normal process.
	Normal ProcessType = iota
	// Error is error process.
	Error
	// Final is final process.
	Final
)

// Init task id.
const Init = "Init"

// ProcessType is type of process.
type ProcessType int

// CfgInfo is config file information.
type CfgInfo struct {
	Path string
	Init bool
	Cfg
}

// Cfg is gcon config file struct.
type Cfg struct {
	Tasks []Task `mapstructure:"tasks"`
}

// CfgInfos is Array of CfgInfo.
type CfgInfos []CfgInfo

// Task is execute task information.
type Task struct {
	// ID is task id.
	ID     string  `mapstructure:"id"`
	Normal Process `mapstructure:"normal"`
	Error  Process `mapstructure:"error"`
	Final  Process `mapstructure:"final"`
}

// TaskInfo is task file and task id.
type TaskInfo struct {
	Path    string
	ID      string
	ProType ProcessType
}

// Func is function struct.
type Func struct {
	Name string `mapstructure:"func"`
	Args Args   `mapstructure:"args"`
}

// Args is func arg.
type Args map[interface{}]interface{}

// Process is array of Func.
type Process []Func

// Funcs is func map.
type Funcs map[string]F

// F is func and arg struct.
type F struct {
	f func(*Gcon, Args) (TaskInfo, error)
	p func(*Gcon, Args, interface{}) error
}

// Store store string data.
type Store map[string]interface{}

// Gcon is gcon app main struct.
type Gcon struct {
	// Now CfgIngo.
	Ci CfgInfo
	// Now TaskInfo.
	Ti TaskInfo

	Store
}

var (
	startCfgFile string
	startTaskID  string

	funcs      = make(Funcs)
	cfgInfos   = make(CfgInfos, 0)
	cfgInfosMu = new(sync.Mutex)
	startGcon  = Gcon{Store: make(Store)}
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "gcon",
	Short: "gcon is 'Go Controller'",
	Long: `gcon executes processing according to the config file.
For example, Log func is executed twice when the following config file is used.

  tasks:
    - id: Startup
      normal:
        - func: Log
          args:
            msg: log 1
            stdout: true
        - func: Log
          args:
            msg: log 2

You can specify the configuration file to use with the arguments,
and the execution task.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	RunE: runE,
}

func (p ProcessType) String() string {
	switch p {
	case Normal:
		return "Normal"
	case Error:
		return "Error"
	case Final:
		return "Final"
	}

	return ""
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(ExitNG)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&startCfgFile, "cfg", "", "config file (default is $HOME/.gcon.toml)")
	RootCmd.PersistentFlags().StringVar(&startTaskID, "task", "Startup", "Start up task id")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	viper.SetConfigName(".gcon")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	if startCfgFile != "" {
		viper.SetConfigFile(startCfgFile)
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Use config file:", viper.ConfigFileUsed())
		startCfgFile = viper.ConfigFileUsed()
	} else {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}
	ci, err := startGcon.importCfg(startCfgFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}
	startGcon.Ci = ci

	// Store cfg info.
	err = startGcon.setCfg(ci.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}

}

func runE(cmd *cobra.Command, args []string) error {

	cmd.SilenceUsage = true

	ti := TaskInfo{
		ID:      startTaskID,
		Path:    startCfgFile,
		ProType: Normal,
	}
	startGcon.setTaskInfo(ti)

	err := startGcon.Engine(ti)
	if err != nil {
		return err
	}

	return nil
}

// Engine execute Process.
func (g *Gcon) Engine(ti TaskInfo) error {

	// Restore previous TaskInfo.
	prevTi := g.Ti
	defer g.setTaskInfo(prevTi)
	// Restore previous CfgInfo.
	prevCi := g.Ci
	defer g.setCfgInfo(prevCi)

	// Get now CfgInfo by TaskInfo.
	ci, err := g.getCfgInfo(ti)
	if err != nil {
		return err
	}

	if ti.Path != "" {
		ti.Path = ci.Path
	}

	// Set now CfgInfo and TaskInfo.
	g.setCfgInfo(ci)
	g.setTaskInfo(ti)

	// Execute final process.
	if ti.ProType == Normal {
		fTi := g.Ti
		fTi.ProType = Final
		defer g.Engine(fTi)
	}

	// Execute Init Task.
	err = g.init()
	if err != nil {
		return err
	}

	// Get process.
	ps, err := g.getProcess()
	if err != nil {
		if ti.ProType == Normal {
			return err
		}
		return nil
	}

	// Execute process.
	for _, f := range ps {
		fmt.Printf("Path: [%v] ID: [%v] Type: [%v] Func: [%v] Args: [%v]\n", ti.Path, ti.ID, ti.ProType, f.Name, f.Args)
		next, err := funcs[f.Name].f(g, f.Args)
		if err != nil && ti.ProType == Normal {
			errTi := ti
			errTi.ProType = Error
			err2 := g.Engine(errTi)
			if err2 != nil {
				return errors.Wrap(err, err2.Error())
			}
			return err
		}
		if next.ID != "" {
			return g.Engine(next)
		}
	}

	return nil
}

// ParseArgs make map to struct and check cfg.
func ParseArgs(args Args, ptr interface{}) error {

	t, err := yaml.Marshal(args)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(t, ptr)
	if err != nil {
		return err
	}

	var check func(reflect.Value) error
	check = func(rv reflect.Value) error {

		switch rv.Kind() {
		case reflect.Ptr:
			rv = rv.Elem()
		case reflect.Slice:
			for i := 0; i < rv.Len(); i++ {
				err := check(rv.Index(i))
				if err != nil {
					return err
				}
			}
		}

		rt := rv.Type()

		for i := 0; i < rv.NumField(); i++ {
			fieldV := rv.Field(i)
			fieldT := rt.Field(i)
			req := isRequire(fieldT.Tag.Get("require"))

			switch fieldV.Kind() {
			case reflect.Ptr, reflect.Struct:
				err := check(fieldV)
				if err != nil {
					return err
				}
			case reflect.Slice:
				for i := 0; i < fieldV.Len(); i++ {
					err := check(fieldV.Index(i))
					if err != nil {
						return err
					}
				}
			case reflect.Int:
				if req && fieldV.Int() == 0 {
					return fmt.Errorf("[%v.%v] is require. but not set", rt.Name(), fieldT.Name)
				}
			case reflect.String:
				if req && fieldV.String() == "" {
					return fmt.Errorf("[%v.%v] is require. but not set", rt.Name(), fieldT.Name)
				}
			case reflect.Bool:
			default:
				return fmt.Errorf("[%v] is not support", fieldV.Kind())
			}
		}

		return nil
	}

	return check(reflect.ValueOf(ptr))
}

func (g *Gcon) setCfg(cfgFile string) error {

	cfgPI, err := file.GetPathInfo(cfgFile)
	if err != nil {
		return err
	}
	g.set("CFG_ID", startTaskID, false)
	g.set("CFG_DIR", cfgPI.Dir, false)
	g.set("CFG_FILE", cfgPI.File, false)
	g.set("CFG_NAME", cfgPI.Name, false)
	g.set("CFG_FILENAME", cfgPI.FileName, false)

	return nil
}

func (g *Gcon) getProcess() (Process, error) {

	// Get task.
	for _, task := range g.Ci.Tasks {
		if task.ID == g.Ti.ID {
			switch g.Ti.ProType {
			case Normal:
				return task.Normal, nil
			case Error:
				return task.Error, nil
			case Final:
				return task.Final, nil
			default:
				return Process{}, fmt.Errorf("Invalid ProcessType [%v]", g.Ti.ProType)
			}
		}
	}

	return Process{}, fmt.Errorf("Not found process. task id: [%v] task file: [%v]", g.Ti.ID, g.Ti.Path)
}

func (g *Gcon) getCfgInfo(ti TaskInfo) (CfgInfo, error) {

	if ti.Path == g.Ci.Path {
		return g.Ci, nil
	}

	// Search now CfgInfo.
	ci, err := g.searchCfgInfo(ti)
	if err != nil {
		// Import cfg file.
		ci, err := g.importCfg(ti.Path)
		if err != nil {
			return CfgInfo{}, err
		}
		return ci, nil
	}

	return ci, nil
}

func (g *Gcon) importCfg(file string) (CfgInfo, error) {

	cfgInfosMu.Lock()
	defer cfgInfosMu.Unlock()

	if file != "" {
		viper.SetConfigFile(file)
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Import config file:", viper.ConfigFileUsed())
	} else {
		return CfgInfo{}, err
	}

	// Load and store cfg.
	cfg := Cfg{}
	err := viper.Unmarshal(&cfg)
	if err != nil {
		return CfgInfo{}, err
	}

	// Check config file.
	err = g.checkCfg(cfg)
	if err != nil {
		return CfgInfo{}, err
	}

	// Set cfgInfo to cfgInfos.
	ci := CfgInfo{
		Path: viper.ConfigFileUsed(),
		Init: false,
		Cfg:  cfg,
	}
	cfgInfos = append(cfgInfos, ci)

	return ci, nil
}

func (g *Gcon) searchCfgInfo(ti TaskInfo) (CfgInfo, error) {

	// Search now CfgInfo.
	for _, task := range g.Ci.Tasks {
		if task.ID == ti.ID {
			return g.Ci, nil
		}
	}

	// Search other CfgInfo.
	cfgInfosMu.Lock()
	defer cfgInfosMu.Unlock()

	for _, ci := range cfgInfos {
		if ci.Path == ti.Path {
			return ci, nil
		}
		for _, task := range ci.Tasks {
			if task.ID == ti.ID {
				return ci, nil
			}
		}
	}

	return CfgInfo{}, fmt.Errorf("Not found task ID: [%v] Path: [%v] ", ti.ID, ti.Path)
}

func (g *Gcon) set(key string, value interface{}, log bool) {

	if log {
		fmt.Printf("[%v] = [%v]\n", key, value)
	}
	g.Store[key] = value
}

func (g *Gcon) get(key string) (interface{}, error) {

	if v, ok := g.Store[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("Not found key: [%v]", key)
}

func (g *Gcon) checkCfg(cfg Cfg) error {

	// Check func.
	check := func(ps Process) error {
		for _, f := range ps {
			// Check func name.
			fn, ok := funcs[f.Name]
			if !ok {
				return fmt.Errorf("func [%v] is not found", f.Name)
			}
			// Check args.
			err := fn.p(g, f.Args, nil)
			if err != nil {
				return err
			}
		}

		return nil
	}

	// Check all task.
	for _, task := range cfg.Tasks {
		// Check normal process.
		err := check(task.Normal)
		if err != nil {
			return err
		}
		// Check error process.
		err = check(task.Error)
		if err != nil {
			return err
		}
		// Check final process.
		err = check(task.Final)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Gcon) init() error {

	if g.Ci.Init {
		return nil
	}

	cfgInfosMu.Lock()

	for i, ci := range cfgInfos {
		if ci.Path == g.Ci.Path {
			cfgInfos[i].Init = true
			g.Ci.Init = true
			cfgInfosMu.Unlock()
			ti := g.Ti
			ti.ID = Init
			return g.Engine(ti)
		}
	}

	cfgInfosMu.Unlock()
	return fmt.Errorf("Not found init task. task file: [%v]", g.Ci.Path)
}

func (g *Gcon) setTaskInfo(ti TaskInfo) {

	pi, _ := file.GetPathInfo(ti.Path)
	g.set("TASK_ID", ti.ID, false)
	g.set("TASK_DIR", pi.Dir, false)
	g.set("TASK_FILE", pi.File, false)
	g.set("TASK_NAME", pi.Name, false)
	g.set("TASK_FILENAME", pi.FileName, false)

	g.set("PROCESS_TYPE", ti.ProType, false)

	g.Ti = ti

}

func (g *Gcon) setCfgInfo(ci CfgInfo) {

	g.Ci = ci
}

func isRequire(req string) bool {

	switch req {
	case "true":
		return true
	case "false":
		return false
	default:
		err := fmt.Sprintf("require tag must be [true] or [false], but set [%v]", req)
		fmt.Fprintln(os.Stderr, err)
		panic(err)
	}

}
