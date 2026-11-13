# Contributing to NTT

:+1::tada: First off, thanks for taking the time to contribute! :tada::+1:

We welcome contributions to this project of any kind including documentation, plugins,
organization, tutorials, bug reports, issues, feature requests, feature
implementations, pull requests, answering questions on the mailing list, helping
to manage issues, etc.

The first contribution can be scary, but there's no need to worry. Everyone has
to start somewhere and we are all nice people who find it great when someone is
interested in our project.


## Give us a star

Stars help us grow. If you like this project, please give us a star :blush:

[![Star on GitHub](https://img.shields.io/github/stars/jonsn0w/hyde.svg?style=social)](https://github.com/nokia/ntt/stargazers)


## Reporting Issues

If you believe you have found an issue in ntt, please use the GitHub issue tracker
to report the problem. If you're not sure if it's a bug or not, start by asking.


## Asking Questions

Our mailing list [ntt@groups.io](mailto:ntt@groups.io) and [GitHub
Discussions](https://github.com/nokia/ntt/discussions) are great places for
asking questions and having discussions.


## Contributing Code

Use [GitHub pull requests](https://docs.github.com/en/get-started/quickstart/github-flow) to contribute code.

If you are doing this the first time you'll probably have to setup your
environment. There are many options, you can use [GitHub
Codespaces](https://docs.github.com/en/codespaces) and develop in your browser
remotely. Or you install everything on your machine and develop locally:


### Using AI

The use of AI tools is generally permitted. However, AI-generated code is not
free: someone must review, understand, test, and maintain it over the long
term. For this reason, please avoid submitting large AI-generated changesets
that cannot reasonably be reviewed.

Please also keep fair use and licensing obligations in mind. If you incorporate
code from other projects—whether directly or indirectly through an AI
assistant—ensure that the original license permits such use. When appropriate,
provide attribution to the authors and contributors whose work you are building
upon.

For significant or critical code sections, consider documenting that AI tools
were involved in their creation. This can help clarify provenance and may
reduce complications related to intellectual property, copyright, or patent
considerations.

### Installing Go

Install Go using your package manager or a manual installation as [described
here](https://go.dev/doc/install).  
You don't always need the latest version of Go, but you should not use versions
older than two years either.


### Installing Visual Studio Code

We suggest you install [Visual Studio Code](https://code.visualstudio.com/docs).  
This is not a hard requirement, you can develop in any editor you like; Vim for
example works great, but we find Visual Studio Code is the easiest to start
with, especially when you are developing with a Microsoft Windows System.


### Installing Git

We use Git for version control and collaborative development. Make sure you
have [Git installed](https://github.com/git-guides/install-git) and [configured
properly](https://docs.github.com/en/get-started/quickstart/set-up-git)(e.g.
user name, email address, GitHub access, ...).


### What you should know about Go

This project's main programming language is Go. If you are new to Go, you'll
find an introduction [here](https://go.dev/learn/). If you are not sure about
the coding style you might find [Effective Go](https://go.dev/doc/effective_go)
and [Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) helpful.


### A Pull Request Workflow

Some basic understanding of how to use Git will be of great use. If you know any good
tutorials, we'd love to link them here.

For most steps I'll assume you will use a Linux environment, but steps should
be similar for MacOS or Windows users. If you have difficulties don't hesitate
to ask; we'll help gladly.

Now it's time to get down to business! Please [Fork][fork] and then clone the
repository. The process is described
[here](https://docs.github.com/en/get-started/quickstart/fork-a-repo) in good
detail.

After you added the upstream remote you can configure git to pull from
upstream, to push to your origin repository. This is optional, but I find it
quite comfortable:

	# Change the remote for from origin to upstream. Pull will use `upstream`:
	git config branch.master.remote upstream

	# Use `origin` for pushing changes.
	git config branch.master.pushRemote origin


Now you're ready to contribute:

0. Create a new branch: `git checkout -b my-branch-name`. The branch name is
   not important, for example choose something simple like
   `fix-missing-imports`.
0. Make your change and remember to add tests. Write initial tests first; this
   makes it easier to become familiar the code base.
0. It's a good habit to run all tests locally before creating a pull request:
   `go run ./...`
0. Push to your fork and [submit a pull request][pr]. When you push GitHub will
   conveniently print the URL for creating a PR on the console. Create a Draft
   PR, when your contribution is not ready yet. Write a [good
   description](https://chris.beams.io/posts/git-commit/) so we know what the
   pull request is about.
0. Pat your self on the back and wait for your pull request to be reviewed and
   merged. We will review any contribution timely. If you don't get any
   reaction, please poke us because then we might have overlooked your PR are
   very sorry about that.

**Aftermath and a new Beginning**

When your request has been merged and you want to create another pull request, don't forget to:

0. checkout the `master` branch again.
0. remove the local (`git branch -d my-branch-name`) and remote feature branch.
0. pull the changes from upstream.
0. synchronize your remote origin repository.


**Tips**

Here are a few things you can do that will increase the likelihood of your pull
request being accepted:

* Write tests.
* Format your code.
* Keep your change as focused as possible. If there are multiple changes you
  would like to make that are not dependent upon each other, submit them as
  separate pull requests.
* Write [good commit messages](https://chris.beams.io/posts/git-commit/).


## Repository Organization

```
ntt                 main binary with sub-commands (list, build, run, lint, ...)
│
├── project         test suite configuration package
├── control         test management and control
│   ├── k3r         runner implementation for k3r
│   └── k3s         runner implementation for k3s
│
├── runtime         runtime system
├── builtins        predefined and builtin functions
├── interpreter     tree walking interpreter for TTCN-3
│
├── ttcn3           language support (semantics and convenience functions)
│   ├── ast         abstract syntax tree and helpers
│   ├── parser      parser (TTCN-3:2018, various extensions, support for Titan, k3 and mtc)
│   ├── scanner     tokenizer
│   ├── token       token types
│   ├── printer     obsolete pretty printer
│   └── doc         documentation tags package
│
├── internal
│   ├── compdb      compilation database types (compile_commands.json)
│   ├── results     result database types (test_results.json)
│   ├── env         environment file handling (ntt.env)
│   ├── loc         source location package
│   ├── log         logging library
│   ├── proc        subprocess library
│   ├── session     session handling library
│   ├── errors      multi-error implementation
│   ├── fs          file caching and filesystem utilities
│   ├── cache	    path cache package (NTT_CACHE, VPATH)
│   ├── memoize     data caching library
│   ├── lsp         language server
│   ├── pipeconn    net.Conn implementation for os.Stdin/os.Stdout (unsed)
│   └── yaml        YAML support library
│
└── k3              k3 support packages
    └── log         log file parser
```

## Issue and Pull Request Labels

Labels help us track and manage issues and pull requests.

| Label Name         |                                    | Description
| ------------------ | ---------------------------------- | -----------
| `enhancement`      | [search][search-label-enhancement] | Feature Requests
| `bug`              | [search][search-label-bug]         | Something isn't working
| `duplicate`        | [search][search-label-duplicate]   | This issue or pull request already exists
| `good first issue` | [search][search-label-first]       | Good for newcomers
| `help wanted`      | [search][search-label-help]        | Extra attention is needed
| `invalid`          | [search][search-label-invalid]     | This doesn't seem right
| `question`         | [search][search-label-question]    | Further information is requested
| `wontfix`          | [search][search-label-wontfix]     | This will not be worked on

## Releasing

Besides source we also provide pre-built binaries. Those binary releases are
built using [GoReleaser](https://goreleaser.com/). 

Everything is automated, if you want to release ntt, just push a git-tag to
this repository. Have a look at existing tags to see how we name things.

When your git-tag went through CI successfully, you'll find a new draft in the
[releases section](https://github.com/nokia/ntt/releases). Edit this draft,
select your tag and write some nice release notes.  

Release notes should be relevant to our users:

* Describe what the new feature is used for and what's problems is solves.
* Add screenshots or screencasts to clarify.
* When breaking compatibility explain why and how the user can fix resulting
  issues.
* Give shoutouts to our contributors, because we are a community!

Again, have a look at previous releases to get some inspiration what to write.

**Dry run**
If you want to release ntt _manually_, install
[goreleaser](https://goreleaser.com/) and try a dry-run first:

	$ goreleaser --snapshot --skip-publish --rm-dist


## Resources

* [How to Contribute to Open Source](https://opensource.guide/how-to-contribute/)
* [Using Pull Requests](https://help.github.com/articles/about-pull-requests/)
* [GitHub Help](https://help.github.com)


[fork]: https://github.com/nokia/ntt/fork
[pr]: https://github.com/nokia/ntt/compare
[code-of-conduct]: CODE_OF_CONDUCT.md
[search-label-enhancement]: https://github.com/nokia/ntt/labels/enhancement
[search-label-duplicate]: https://github.com/nokia/ntt/labels/duplicate
[search-label-first]: https://github.com/nokia/ntt/labels/good%20first%20issue
[search-label-help]: https://github.com/nokia/ntt/labels/help%20wanted
[search-label-invalid]: https://github.com/nokia/ntt/labels/invalid
[search-label-question]: https://github.com/nokia/ntt/labels/question
[search-label-wontfix]: https://github.com/nokia/ntt/labels/wontfix
