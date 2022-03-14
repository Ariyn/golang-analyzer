package sample

func sampleFunc1() {
	getB()
}

var sampleFunc2 = func() {
	sampleFunc3 := func() {}

	sampleFunc3()
}

type sampleFunc4 func()

var sampleFunc5 = func() {}
