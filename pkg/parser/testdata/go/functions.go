// Test file for Go functions and methods
package testdata

import "fmt"

// SimpleFunction is a function with no parameters
func SimpleFunction() {
	fmt.Println("Simple")
}

// Add adds two integers
func Add(a int, b int) int {
	return a + b
}

// ProcessData processes a slice of data
func ProcessData(data []int) {
	for i := range data {
		data[i] *= 2
	}
}

// FunctionWithMultipleReturns demonstrates multiple return values
func FunctionWithMultipleReturns(x int) (int, error) {
	if x < 0 {
		return 0, fmt.Errorf("negative value")
	}
	return x * 2, nil
}

// Calculator represents a calculator
type Calculator struct {
	value int
}

// Add is a method that adds to the calculator value
func (c *Calculator) Add(x int) {
	c.value += x
}

// Multiply is a method with a pointer receiver
func (c *Calculator) Multiply(x int) int {
	c.value *= x
	return c.value
}

// GetValue is a method with a value receiver
func (c Calculator) GetValue() int {
	return c.value
}

// NewCalculator is a constructor function
func NewCalculator() *Calculator {
	return &Calculator{value: 0}
}

// Compute calls other functions
func Compute(x, y int) int {
	sum := Add(x, y)
	SimpleFunction()
	calc := NewCalculator()
	calc.Add(sum)
	result := calc.Multiply(2)
	return result
}
