package format

// Options configures the wrapping printer.
//
// The fields are the public knobs the LSP, the CLI and the
// per-project manifest all populate. They are intentionally narrow:
// only options that change observable output land here.
type Options struct {
	// PrintWidth is the soft right margin in columns. Groups whose
	// flattened form does not fit before this column are broken
	// vertically. Defaults to 100.
	PrintWidth int

	// TabWidth is how many columns a tab character counts for. Used
	// both when measuring the flat form against PrintWidth and when
	// emitting indentation in space-mode.
	TabWidth int

	// UseSpaces selects between tab and space indentation. Defaults to
	// tabs to preserve the historical behaviour of the canonical
	// printer.
	UseSpaces bool

	// MaxEmptyLines caps how many consecutive empty lines we preserve
	// between top-level declarations. Zero disables the cap.
	MaxEmptyLines int
}

// DefaultOptions returns Options matching the historical canonical
// printer defaults. New consumers should generally start from these and
// override only what they care about.
func DefaultOptions() Options {
	return Options{
		PrintWidth:    100,
		TabWidth:      8,
		UseSpaces:     false,
		MaxEmptyLines: 1,
	}
}
