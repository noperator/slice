// Test file for C++ classes, methods, and functions
#include <iostream>

// Simple top-level function
void simpleFunction() {
    std::cout << "Simple" << std::endl;
}

// Function with parameters
int add(int a, int b) {
    return a + b;
}

// Calculator class
class Calculator {
private:
    int value;

public:
    // Constructor
    Calculator() : value(0) {}

    // Method with parameter
    void add(int x) {
        value += x;
    }

    // Method with return value
    int multiply(int x) {
        value *= x;
        return value;
    }

    // Getter method
    int getValue() const {
        return value;
    }
};

// Function that uses class and calls other functions
int compute(int x, int y) {
    int sum = add(x, y);
    Calculator calc;
    calc.add(sum);
    int result = calc.multiply(2);
    simpleFunction();
    return result;
}

// Main function
int main() {
    int result = compute(5, 10);
    std::cout << "Result: " << result << std::endl;
    return 0;
}
