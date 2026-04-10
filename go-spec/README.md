# Go Spec

This directory serves as a cache/parsed and possibly augmented or indexed source of reference for the Golang spec, found online at https://go.dev/ref/spec

The Go spec is quite large in size, about 50k OpenAI tokens or there abouts, so we're caching it and splitting it into smaller files so that LLMs (and humans, honestly - it's a big file) can access it in smaller chunks. 

Note that the Golang spec doesn't really tell you EVERYTHING there is to not about Go. The source for Go's spec, Go itself, and some of the tools or other stuff used by the Go project, can be found at https://cs.opensource.google/go with the actual compiler and core logic in https://cs.opensource.google/go/go

This spec is accurate as of Go 1.26 and was read from https://cs.opensource.google/go/go/+/master: on April 9, 2026. Note that the other files in the same directory in the actual Go source repo (currently asm.html - go's assembler; go_mem.html - go's memory model; godebug.md - a go setting for managing breaking changes and compat issues across go versions; README.md - mostly describes the process of creating release notes, irrelevant to those not working directly on Go; initial/ - template for release notes; next/ - new and upcoming changes to go in its next version, very interesting but not that relevant to most people, but good place to learn about what's coming up because it's very structured and descriptive) may also be of interest.

The layout here is as follows: all <h2> from go's spec become a file like Introduction.html, including the h2 tag itself. The html tags are left as-is because they are organized very neatly/minimally already. We also extract h3 tags and their contents into eg Expressions.Operands.html. Later we plan to convert these to markdown, mostly for compatibility with other tools for project documentation.
