package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

type StringInt struct {
	string
}

func (i *StringInt) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		i.string = "\"" + s + "\""
		return nil
	}

	var integer int
	if err := json.Unmarshal(data, &integer); err == nil {
		i.string = strconv.Itoa(integer)
		return nil
	}

	return &json.MarshalerError{Err: fmt.Errorf("%s", data)}
}

var (
	component string
	args      []string
	jsonargs  string
)

type mainTmpl struct {
	FuncName string
	Args     string
}

func init() {
	flag.StringVar(&component, "templ", "", "compiled templ go code")
	flag.StringVar(&jsonargs, "data", "", "arguments in json format")
	flag.Parse()
}

func main() {
	if component == "" {
		panic(fmt.Errorf("no component file passed"))
	}

	if jsonargs != "" {
		data, err := os.ReadFile(jsonargs)
		if err != nil {
			panic(fmt.Errorf("os.ReadFile: %w", err))
		}

		var arg []StringInt

		if err := json.Unmarshal(data, &arg); err != nil {
			panic(fmt.Errorf("json.Unmarshal: %w", err))
		}

		args = make([]string, len(arg))
		for idx, v := range arg {
			args[idx] = v.string
		}
	}

	if err := runTempl(); err != nil {
		panic(fmt.Errorf("runTempl: %w", err))
	}
}

func runTempl() error {
	files, cleanupFn, err := generateExec()
	if err != nil {
		return fmt.Errorf("generateExec: %w", err)
	}
	defer cleanupFn()

	buildArgs := append([]string{"run"}, files...)

	cmd := exec.Command("go", buildArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cmd.Start: %w", err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	done := make(chan struct{})
	go func(done chan struct{}, cmd *exec.Cmd) {
		cmd.Wait()
		done <- struct{}{}
	}(done, cmd)

	select {
	case <-done:
	case sig := <-sigc:
		cmd.Process.Signal(sig)
		<-done
	}
	return nil
}

func generateExec() (execFilePath []string, cleanup func(), err error) {
	tempDir := getTempDir()
	componentFilename, err := createComponentClone(tempDir)
	if err != nil {
		return nil, nil, fmt.Errorf("createComponentClone: %w", err)
	}

	mainFile, err := os.CreateTemp(tempDir, "main*.go")
	if err != nil {
		return nil, nil, fmt.Errorf("error Creating temp file: os.CreateTemp: %w", err)
	}
	defer mainFile.Close()

	tmpl, err := template.New("main.tmpl").ParseFiles("main.tmpl")
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing main template: %w", err)
	}

	data := mainTmpl{
		Args:     strings.Join(args, ", "),
		FuncName: getFnName(component),
	}

	if err := tmpl.Execute(mainFile, data); err != nil {
		return nil, nil, fmt.Errorf("template.Execute: %w", err)
	}

	return []string{componentFilename, mainFile.Name()}, func() {
		os.RemoveAll(tempDir)
	}, nil
}

func getFnName(filename string) string {
	filename = filepath.Base(filename)
	idx := strings.LastIndex(filename, "_")
	return filename[:idx]
}

func createComponentClone(dir string) (filename string, err error) {
	base := filepath.Base(component)
	name := base[:strings.LastIndex(base, ".")]
	componentFile, err := os.CreateTemp(dir, name+"*.go")
	if err != nil {
		return "", fmt.Errorf("error creating tempfile: os.CreateTemp: %w", err)
	}
	defer componentFile.Close()

	src, err := os.Open(component)
	if err != nil {
		return "", fmt.Errorf("error opening component: os.Open: %w", err)
	}
	defer src.Close()

	if _, err := io.Copy(componentFile, src); err != nil {
		return "", fmt.Errorf("error copying data from compontent to tempFile: io.Copy: %s", err)
	}

	return componentFile.Name(), nil
}

func getTempDir() string {
	dirName := "go_templ"
	dir, err := os.MkdirTemp("", dirName)
	if err != nil {
		panic(err)
	}

	return dir
}
