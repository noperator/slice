/* Small graph: entry -> middle -> leaf -> middle; sibling shares entry. */
void leaf(void) {
    middle();
}

void middle(void) {
    leaf();
}

void sibling(void) {
    ;
}

void entry(void) {
    middle();
    sibling();
}

static void duplicate(void) {
    ;
}
