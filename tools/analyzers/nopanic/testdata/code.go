package testdata

func A() error {
	panic("feeling cute, let's panic!") // want "panic\\(\\) should not be used, except in rare situations or init functions"
}

func B(foo interface{}) error {
	if foo == nil {
		panic("impossible condition: foo is nil") //lint:nopanic -- This is validated by the caller.
	}

	if _, ok := foo.(string); !ok {
		panic("foo should not be a string!!") // want "panic\\(\\) should not be used, except in rare situations or init functions"
	}

	return nil
}

//lint:nopanic -- This is method is really safe ;)
func C(foo interface{}) error {
	if foo == nil {
		panic("impossible condition: foo is nil")
	}

	return nil
}
