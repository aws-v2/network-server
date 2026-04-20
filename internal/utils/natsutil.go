package utils

import "fmt"

type SubjectBuilder struct {
	Prefix  string // dev, staging, prod
	Service string // network, iam, compute
	Version string // v1
}

func (s SubjectBuilder) Build(parts ...string) string {
	base := fmt.Sprintf("%s.%s.%s", s.Prefix, s.Service, s.Version)
	for _, p := range parts {
		base += "." + p
	}
	return base
}



func BuildSubject(prefix string, service string, parts ...string) string {
	base := prefix + "." + service
	for _, p := range parts {
		base += "." + p
	}
	return base
}



