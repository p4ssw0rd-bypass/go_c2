package main

import "embed"

//go:embed public/*
var embeddedStaticFS embed.FS