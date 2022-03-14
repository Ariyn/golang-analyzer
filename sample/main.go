package sample

import (
	"log"
	"os"
	t "strings"

	_ "github.com/ariyn/golang-analyzer/analyzer"
)

func main() {
	log.Println("hello, world!")

	a := 2
	b := getB()
	b2 := getB()
	b3 := getB()

	result := multiply(a, b)

	checkResult(result)
	log.Println(b2, b3)

	sampleFunc1()
}

// getB is test function
// thi sdf
func getB() int { // test
	t.Contains("", "")
	return 3
}

// multiply is test function
func multiply(a, b int) (result int) {
	result = a * b
	return
}

func checkResult(result int) {
	log.Println(result)
}

type x int

func (recv *x) SampleFunction(a, b int, c *string, d bool) (aa, bb int) {
	log.Println(a, b)
	log.Println(c, d)
	aa = a + b
	return a, b
}

func (recv x) SampleFunction2(a, b int, c *string, d bool) (aa, bb int) {
	log.Println(a, b)
	log.Println(c, d)
	aa = a + b
	return a, b
}

func (x) SampleFunction3(a, b int, c *string, d bool) (aa, bb int) {
	log.Println(a, b)
	log.Println(c, d)
	aa = a + b
	return a, b
}

func (*x) SampleFunction4(a, b int, c *string, d bool) (aa, bb int) {
	log.Println(a, b)
	log.Println(c, d)
	aa = a + b
	return a, b
}

func (*x) SampleFunction5(log.Logger, *os.File) (aa, bb int) {
	return
}

func (*x) SampleFunction6(abc log.Logger, ddd *os.File) (aa, bb int) {
	return
}

func (*x) SampleFunction7(abc log.Logger, ddd *os.File) (int, int) {
	return 0, 0
}
