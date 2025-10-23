// Test file for C functions

#include <stdio.h>

// Simple function with no parameters
void simple_function() {
    printf("Simple\n");
}

// Function with parameters
int add(int a, int b) {
    return a + b;
}

// Function with pointer parameters
void process_data(int *data, size_t len) {
    for (size_t i = 0; i < len; i++) {
        data[i] *= 2;
    }
}

// Function that calls other functions
int compute(int x, int y) {
    int sum = add(x, y);
    simple_function();
    return sum;
}

// Main function
int main(int argc, char **argv) {
    int result = compute(5, 10);
    printf("Result: %d\n", result);
    return 0;
}
