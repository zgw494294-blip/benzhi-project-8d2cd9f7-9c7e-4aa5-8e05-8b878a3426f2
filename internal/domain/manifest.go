package domain

import "strings"

func ManifestText(c *ColdChainCase) string { return strings.Join(Manifest(c), "\n") }
