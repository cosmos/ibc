# Config package development rules

1. Follow the following code blocks ordering (excluding test cases):
 - exported interfaces
 - unexported interfaces
 - exported consts
 - unexported consts
 - exported types definitions. Nested types should have child definitions below parent
 - `Default*()` `Declare*() functions
 - exported methods, same order as as types
 - unexported methods
 - exported functions
 - unexported functions