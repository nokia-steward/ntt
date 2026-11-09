package basket

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nokia/ntt/internal/env"

	"github.com/nokia/ntt/project"
	"github.com/spf13/pflag"
)

// Flags returns a flagset with all flags for filtering objects.
//
// Flags are regular expressions to filter objects. If you pass multiple regular
// expressions, all of them must match (AND). Example:
//
//	$ cat example.ttcn3
//	testcase foo() ...
//	testcase bar() ...
//	testcase foobar() ...
//	...
//
//	$ ntt list --regex=foo --regex=bar
//	example.foobar
//
//	$ ntt list --regex='foo|bar'
//	example.foo
//	example.bar
//	example.foobar
//
// Similarly, you can also specify regular expressions for documentation tags.
// Example:
//
//	$ cat example.ttcn3
//	// @one
//	// @two some-value
//	testcase foo() ...
//
//	// @two: some-other-value
//	testcase bar() ...
//	...
//
//	$ ntt list --tags-regex=@one --tags-regex=@two
//	example.foo
//
//	$ ntt list --tags-regex='@two: some'
//	example.foo
//	example.bar
func Flags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("basket", pflag.ContinueOnError)
	fs.StringSliceP("regex", "r", nil, "list objects matching regular * expression.")
	fs.StringSliceP("exclude", "x", nil, "exclude objects matching regular * expresion.")
	fs.StringSliceP("tags-regex", "R", nil, "list objects with tags matching regular * expression")
	fs.StringSliceP("tags-exclude", "X", nil, "exclude objects with tags matching * regular expression")
	return fs
}

// A Basket is a filter for objects. It can be used to filter objects by name
// and tags.
//
// Baskets are also filters defined by environment variables of the form:
//
//	K3_LIST_BASKETS_<name> = <filters>
//
// For example, to define a basket "stable" which excludes all objects with @wip
// or @flaky tags:
//
//	export K3_LIST_BASKETS_stable="-X @wip|@flaky"
//
// Baskets become active when they are listed in colon separated environment
// variable K3_LIST_BASKETS. If you specify multiple baskets, at least of them
// must match (OR).
//
// Rule of thumb: all baskets are ORed, all explicit filter options are ANDed.
// Example:
//
//	$ export K3_LIST_BASKETS_stable="--tags-exclude @wip|@flaky"
//	$ export K3_LIST_BASKETS_ipv6="--tags-regex @ipv6"
//	$ K3_LIST_BASKETS=stable:ipv6 ntt list -R @flaky
//
// Above example will output all tests with a @flaky tag and either @wip or @ipv6 tag.
//
// If a basket is not defined by an environment variable, it's equivalent to a
// "--tags-regex" filter. For example, to lists all tests, which have either a
// @flaky or a @wip tag:
//
//	# Note, flaky and wip baskets are not specified explicitly.
//	$ K3_LIST_BASKETS=flaky:wip ntt list
//
//	# This does the same:
//	$ ntt list --tags-regex="@wip|@flaky"
type Basket struct {
	// Name is the name of the basket. The basket is used to filter objects
	// by tag, if no explicit filters are given.
	Name string

	// Regular expressions the object name must match.
	NameRegex []string

	// Regular expressions the object name must not match.
	NameExclude []string

	// Regular expressions the object tags must match.
	TagsRegex []string

	// Regular expressions the object tags must not match.
	TagsExclude []string

	// Baskets are sub-baskets to be ORed.
	Baskets []Basket
}

// New creates a new basket and parses the given arguments.
func New(name string, args ...string) (Basket, error) {

	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.AddFlagSet(Flags())
	if err := fs.Parse(args); err != nil {
		return Basket{}, err
	}
	return NewWithFlags(name, fs)
}

func NewWithFlags(name string, fs *pflag.FlagSet) (Basket, error) {
	b := Basket{Name: name}

	var err error

	b.NameRegex, err = fs.GetStringSlice("regex")
	if err != nil {
		return b, err
	}

	b.NameExclude, err = fs.GetStringSlice("exclude")
	if err != nil {
		return b, err
	}
	b.TagsRegex, err = fs.GetStringSlice("tags-regex")
	if err != nil {
		return b, err
	}
	b.TagsExclude, err = fs.GetStringSlice("tags-exclude")
	if err != nil {
		return b, err
	}
	return b, nil
}

// Load baskets from given environment variable from environment
// or from configuration.
func (b *Basket) LoadFromEnvOrConfig(c *project.Config, key string) error {
	get := func(name string) string {
		if s, ok := env.LookupEnv(name); ok {
			return s
		}

		if c != nil {
			if c.Variables[name] != "" {
				return c.Variables[name]
			}
			if strings.HasPrefix(name, "NTT") {
				return c.Variables[strings.Replace(name, "NTT", "K3", 1)]
			}
		}
		return ""
	}
	s := get(key)
	if s == "" {
		return nil
	}

	for _, name := range strings.Split(s, ":") {
		// Ignore empty fields
		if name == "" {
			continue
		}
		args := strings.Fields(get(fmt.Sprintf("%s_%s", key, name)))
		if len(args) == 0 {
			args = []string{"-R", "@" + name}
		}

		sb, err := New(name, args...)
		if err != nil {
			return err
		}
		b.Baskets = append(b.Baskets, sb)
	}
	return nil
}

// Match returns true if the given name and tags match the basket or sub-basket filters.
func (b *Basket) Match(name string, tags [][]string) bool {
	ok := b.match(name, tags)
	if len(b.Baskets) == 0 {
		return ok
	}

	for _, basket := range b.Baskets {
		if basket.Match(name, tags) && ok {
			return true
		}
	}
	return false
}

// match returns true if the given name and tags match the basket filters.
func (b *Basket) match(name string, tags [][]string) bool {
	if !b.matchAll(b.NameRegex, name) {
		return false
	}
	if len(b.NameExclude) > 0 && b.matchAll(b.NameExclude, name) {
		return false
	}

	if len(b.TagsRegex) > 0 {
		if len(tags) == 0 {
			return false
		}
		if !b.matchAllTags(b.TagsRegex, tags) {
			return false
		}
	}

	if len(b.TagsExclude) > 0 && b.matchAllTags(b.TagsExclude, tags) {
		return false
	}

	return true
}

// matchAll returns true if all regular expressions match the given string.
func (b *Basket) matchAll(regexes []string, s string) bool {
	for _, r := range regexes {
		if ok, _ := regexp.Match(r, []byte(s)); !ok {
			return false
		}
	}
	return true
}

// matchAllTags returns true if all regular expressions match the all given tags.
func (b *Basket) matchAllTags(regexes []string, tags [][]string) bool {
next:
	for _, r := range regexes {
		f := strings.SplitN(r, ":", 2)
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		for _, tag := range tags {
			if ok, _ := regexp.Match(f[0], []byte(tag[0])); !ok {
				continue
			}
			if len(f) > 1 {
				if ok, _ := regexp.Match(f[1], []byte(tag[1])); !ok {
					continue
				}
			}
			continue next
		}
		return false
	}
	return true
}
