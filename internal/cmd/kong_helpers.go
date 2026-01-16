package cmd

import "github.com/alecthomas/kong"

func flagProvided(kctx *kong.Context, name string) bool {
	if kctx == nil {
		return false
	}
	for _, trace := range kctx.Path {
		if trace.Flag != nil && trace.Flag.Name == name {
			return true
		}
	}
	return false
}

func flagProvidedAny(kctx *kong.Context, names ...string) bool {
	for _, name := range names {
		if flagProvided(kctx, name) {
			return true
		}
	}
	return false
}
