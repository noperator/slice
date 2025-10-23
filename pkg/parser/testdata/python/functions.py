# Test file for Python classes, methods, and functions

# Simple top-level function
def simple_function():
    print("Simple")

# Function with parameters
def add(a, b):
    return a + b

# Function with type hints
def multiply(x: int, y: int) -> int:
    return x * y

# Calculator class
class Calculator:
    def __init__(self):
        self.value = 0

    # Method with parameter
    def add(self, x):
        self.value += x

    # Method with return value
    def multiply_value(self, x):
        self.value *= x
        return self.value

    # Getter method
    def get_value(self):
        return self.value

# Function that uses class and calls other functions
def compute(x, y):
    total = add(x, y)
    calc = Calculator()
    calc.add(total)
    result = calc.multiply_value(2)
    simple_function()
    return result

# Top-level call
if __name__ == "__main__":
    result = compute(5, 10)
    print(f"Result: {result}")
