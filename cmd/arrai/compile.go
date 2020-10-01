package main

import (
	"bytes"
	"context"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/arr-ai/arrai/pkg/arraictx"
	"github.com/arr-ai/arrai/pkg/ctxfs"
	"github.com/arr-ai/arrai/syntax"
	"github.com/spf13/afero"
	"github.com/urfave/cli/v2"
)

const mainTemplate = `
// AUTOGENERATED - DO NOT EDIT
package main

import (
    "context"
    "os"

    "github.com/arr-ai/arrai/pkg/arraictx"
    "github.com/arr-ai/arrai/pkg/arrai"
    "github.com/arr-ai/arrai/syntax"
    "github.com/arr-ai/arrai/tools"
)

func main() {
    // TODO: set //os.args argument without global var assignment
    tools.Arguments = os.Args
    ctx := arraictx.InitRunCtx(context.Background())
    value, err := syntax.EvaluateBundle(ctx, []byte{ {{.Data}} })
    if err != nil {
        panic(err)
    }

    if err := arrai.OutputValue(ctx, value, os.Stdout, ""); err != nil {
        panic(err)
    }
}
`

var compileCommand = &cli.Command{
	Name:    "compile",
	Aliases: []string{"c"},
	Usage:   "compile arrai scripts into a runnable binary",
	Action:  compile,
	Flags: []cli.Flag{
		outFlag,
	},
}

func compile(c *cli.Context) error {
	file := c.Args().Get(0)
	ctx := arraictx.InitCliCtx(context.Background(), c)

	return compileFile(ctx, file, c.Value("out").(string))
}

func compileFile(ctx context.Context, path, out string) error {
	if err := fileExists(ctx, path); err != nil {
		return err
	}

	bundledScripts := bytes.Buffer{}
	if err := bundleFiles(ctx, path, &bundledScripts); err != nil {
		return err
	}

	if out == "" {
		out = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	f, err := ctxfs.SourceFsFrom(ctx).Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := buildBinary(ctx, bundledScripts.Bytes(), f); err != nil {
		return err
	}
	return nil
}

func createGoFile(bundledScripts []byte) ([]byte, error) {
	mainFile, err := template.New("main").Parse(mainTemplate)
	if err != nil {
		return nil, err
	}

	data := make([]string, 0, len(bundledScripts))
	for _, b := range bundledScripts {
		data = append(data, strconv.Itoa(int(b)))
	}

	mainGo := bytes.Buffer{}
	if err := mainFile.Execute(&mainGo, struct{ Data string }{strings.Join(data, ",")}); err != nil {
		return nil, err
	}
	return mainGo.Bytes(), nil
}

func buildBinary(ctx context.Context, bundledScripts []byte, out afero.File) error {
	goFile, err := createGoFile(bundledScripts)
	if err != nil {
		return err
	}

	_, module, err := syntax.GetModuleFromBundle(ctx, bundledScripts)
	if err != nil {
		return nil
	}

	fs := ctxfs.SourceFsFrom(ctx)

	buildDir, err := afero.TempDir(fs, "", path.Base(module))
	if err != nil {
		return err
	}
	defer func() {
		if err := fs.RemoveAll(buildDir); err != nil {
			panic(err)
		}
	}()

	mainFilePath := filepath.Join(buildDir, "main.go")
	f, err := fs.Create(mainFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Write(goFile); err != nil {
		return err
	}

	cmds := []*exec.Cmd{
		exec.CommandContext(ctx, "go", "mod", "init", module),
		exec.CommandContext(ctx, "go", "mod", "tidy"),
		exec.CommandContext(ctx, "go", "build", "-o", "main", mainFilePath),
	}

	for _, c := range cmds {
		c.Dir = buildDir
		if err = c.Run(); err != nil {
			return err
		}
	}

	file, err := ctxfs.ReadFile(fs, filepath.Join(buildDir, "main"))
	if err != nil {
		return err
	}

	if _, err = out.Write(file); err != nil {
		return err
	}

	// 0751 for rwxr-x--x the same as golang binary
	return fs.Chmod(out.Name(), 0751)
}
