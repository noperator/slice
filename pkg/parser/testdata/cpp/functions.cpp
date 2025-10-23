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

    // Destructor
    ~Calculator() {}

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

    // Operator overloads
    bool operator==(const Calculator &other) const {
        return value == other.value;
    }

    bool operator<(const Calculator &other) const {
        return value < other.value;
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

// Test class with methods defined outside
class ExternalMethods {
public:
    void externalMethod(int x);
    ~ExternalMethods();
    bool operator!=(const ExternalMethods &other) const;
};

// Method defined outside class body
void ExternalMethods::externalMethod(int x) {
    std::cout << "External: " << x << std::endl;
}

// Destructor defined outside class body
ExternalMethods::~ExternalMethods() {
    // cleanup
}

// Operator defined outside class body
bool ExternalMethods::operator!=(const ExternalMethods &other) const {
    return true;
}
