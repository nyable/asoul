package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"asoul/internal/diffview"
	"asoul/internal/i18n"
	"asoul/internal/progress"

	"github.com/mattn/go-isatty"
)

// Output manages CLI stdout/stderr outputs, JSON serialization, and verbosity.
type Output struct {
	stdout         io.Writer
	stderr         io.Writer
	json           bool
	quiet          bool
	noColor        bool
	verbose        bool
	nonInteractive bool
	yes            bool
}

// NewOutput creates an Output manager based on flags.
func NewOutput() *Output {
	return &Output{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// SetWriters sets custom stdout and stderr writers.
func (o *Output) SetWriters(stdout, stderr io.Writer) {
	if stdout != nil {
		o.stdout = stdout
	}
	if stderr != nil {
		o.stderr = stderr
	}
}

// IsTTY returns true if stdout is connected to a terminal.
func (o *Output) IsTTY() bool {
	if f, ok := o.stdout.(*os.File); ok {
		return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return false
}

// PrintJSON writes indented JSON directly to stdout. Nil slices and maps are
// normalized to empty arrays/objects so that empty results are always valid,
// predictable JSON (never null).
func (o *Output) PrintJSON(v interface{}) error {
	enc := json.NewEncoder(o.stdout)
	enc.SetIndent("", "  ")
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return enc.Encode(v)
	}
	return enc.Encode(normalizeJSONValue(rv).Interface())
}

// EmitError emits a structured JSON error document when in JSON mode (always a
// single valid document, even for failures) and returns err so the caller
// exits non-zero. In human mode the error is returned for Cobra to print.
func (o *Output) EmitError(code string, err error) error {
	if err == nil {
		return nil
	}
	if o.json {
		doc := map[string]interface{}{"error": err.Error()}
		if code != "" {
			doc["code"] = code
		}
		_ = o.PrintJSON(doc)
	}
	return err
}

// normalizeJSONValue recursively replaces nil slices with empty slices and nil
// maps with empty maps so JSON encoders never emit null for collections.
func normalizeJSONValue(rv reflect.Value) reflect.Value {
	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return rv
		}
		inner := normalizeJSONValue(rv.Elem())
		out := reflect.New(rv.Type()).Elem()
		out.Set(inner)
		return out
	case reflect.Ptr:
		if rv.IsNil() {
			return rv
		}
		inner := normalizeJSONValue(rv.Elem())
		out := reflect.New(rv.Type().Elem())
		out.Elem().Set(inner)
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return reflect.MakeSlice(rv.Type(), 0, 0)
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out.Index(i).Set(normalizeJSONValue(rv.Index(i)))
		}
		return out
	case reflect.Map:
		if rv.IsNil() {
			return reflect.MakeMap(rv.Type())
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), normalizeJSONValue(iter.Value()))
		}
		return out
	case reflect.Struct:
		out := reflect.New(rv.Type()).Elem()
		out.Set(rv)
		for i := 0; i < rv.NumField(); i++ {
			if f := out.Field(i); f.CanSet() {
				f.Set(normalizeJSONValue(rv.Field(i)))
			}
		}
		return out
	default:
		return rv
	}
}

// ProgressActive reports whether an in-place progress line can be rendered:
// only on an interactive terminal and never for JSON or quiet output.
func (o *Output) ProgressActive() bool {
	return !o.json && !o.quiet && o.IsTTY()
}

// Progress renders an in-place progress line on stdout. It is a no-op when
// ProgressActive is false so machine-readable output never sees control codes.
func (o *Output) Progress(current, total int, label string) {
	if !o.ProgressActive() {
		return
	}
	fmt.Fprintf(o.stdout, "\r\033[K%s", renderCLIProgress(current, total, label))
}

// ClearProgress erases the current progress line.
func (o *Output) ClearProgress() {
	if !o.ProgressActive() {
		return
	}
	fmt.Fprint(o.stdout, "\r\033[K")
}

// ProgressFunc adapts structured service progress into the CLI renderer.
func (o *Output) ProgressFunc() progress.Func {
	if !o.ProgressActive() {
		return nil
	}
	return func(u progress.Update) {
		o.Progress(u.Current, u.Total, cliProgressLabel(u))
	}
}

func renderCLIProgress(current, total int, label string) string {
	const width = 20
	if total <= 0 {
		return fmt.Sprintf("[%s] %s", strings.Repeat("░", width), label)
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	filled := current * width / total
	pct := current * 100 / total
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	if label == "" {
		return fmt.Sprintf("[%s] %d%% (%d/%d)", bar, pct, current, total)
	}
	return fmt.Sprintf("[%s] %d%% (%d/%d) %s", bar, pct, current, total, label)
}

// cliProgressLabel picks the most specific label available: a raw detail for
// unknown phases, a concrete item label for count-only phases, or the
// localized phase name.
func cliProgressLabel(u progress.Update) string {
	switch u.Phase {
	case progress.PhaseUnknown:
		return u.Text
	case progress.PhaseClone, progress.PhaseFetch,
		progress.PhaseCounting, progress.PhaseCompressing,
		progress.PhaseReceiving, progress.PhaseResolving:
		if key := u.Phase.I18nKey(); key != "" {
			return i18n.T(key)
		}
		return u.Text
	}
	if u.Text != "" {
		return u.Text
	}
	if key := u.Phase.I18nKey(); key != "" {
		return i18n.T(key)
	}
	return ""
}

// Println prints to stdout unless quiet mode is active.
func (o *Output) Println(a ...interface{}) {
	if !o.quiet && !o.json {
		fmt.Fprintln(o.stdout, a...)
	}
}

// Printf formats and prints to stdout unless quiet mode is active.
func (o *Output) Printf(format string, a ...interface{}) {
	if !o.quiet && !o.json {
		fmt.Fprintf(o.stdout, format, a...)
	}
}

// Verbosef prints debugging info to stderr if verbose mode is active.
func (o *Output) Verbosef(format string, a ...interface{}) {
	if o.verbose && !o.quiet {
		fmt.Fprintf(o.stderr, "[debug] "+format+"\n", a...)
	}
}

// Errorf prints error message to stderr.
func (o *Output) Errorf(format string, a ...interface{}) {
	fmt.Fprintf(o.stderr, "Error: "+format+"\n", a...)
}

// Warnf prints warning message to stderr.
func (o *Output) Warnf(format string, a ...interface{}) {
	if !o.quiet {
		fmt.Fprintf(o.stderr, "Warning: "+format+"\n", a...)
	}
}

// Successf prints success message to stdout.
func (o *Output) Successf(format string, a ...interface{}) {
	if !o.quiet && !o.json {
		fmt.Fprintf(o.stdout, "✓ "+format+"\n", a...)
	}
}

// Infof prints informational message to stdout.
func (o *Output) Infof(format string, a ...interface{}) {
	if !o.quiet && !o.json {
		fmt.Fprintf(o.stdout, "• "+format+"\n", a...)
	}
}

// PrintDiff formats and prints diff with project-standard syntax styles and top summary.
func (o *Output) PrintDiff(diffText string) {
	if o.quiet || o.json {
		return
	}
	rendered := diffview.Render(diffText, diffview.RenderOptions{
		Color:          !o.noColor && o.IsTTY(),
		IncludeSummary: true,
		Width:          80,
	})
	fmt.Fprintln(o.stdout, rendered)
}
