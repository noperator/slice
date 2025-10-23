// Test file for TypeScript classes, methods, and functions

// Simple top-level function
function simpleFunction(): void {
    console.log("Simple");
}

// Function with parameters and return type
function add(a: number, b: number): number {
    return a + b;
}

// Function with optional parameter
function greet(name: string, greeting?: string): string {
    return `${greeting || "Hello"}, ${name}`;
}

// Calculator class
class Calculator {
    private value: number;

    constructor() {
        this.value = 0;
    }

    // Method with parameter
    add(x: number): void {
        this.value += x;
    }

    // Method with return value
    multiply(x: number): number {
        this.value *= x;
        return this.value;
    }

    // Getter method
    getValue(): number {
        return this.value;
    }
}

// Function that uses class and calls other functions
function compute(x: number, y: number): number {
    const sum = add(x, y);
    const calc = new Calculator();
    calc.add(sum);
    const result = calc.multiply(2);
    simpleFunction();
    return result;
}

// Arrow function
const processData = (data: number[]): void => {
    data.forEach(x => console.log(x * 2));
};

// Export for module
export { simpleFunction, add, compute, Calculator };
