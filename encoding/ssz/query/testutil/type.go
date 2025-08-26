package testutil

type PathTest struct {
	Path     string
	Expected any
}

type TestSpec struct {
	Name      string
	Type      any
	Instance  any
	PathTests []PathTest
}
