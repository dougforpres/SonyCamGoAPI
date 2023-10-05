# SonyCamGoAPI
A go REST server providing access to Sony Camera

This is a simple API that may or may not move forward.
The biggest struggle encountered was calling the various DLL functions. Go supports calling some Windows APIs but seems woefully ill-equipped to call other functions that may require/return various data structures.

## WinStruct
As such, the "winstruct" code was written to provide marshal/unmarshal support. It works in a similar way to the JSON marshaler, and has an additional "windows" value that contains the win32 type of the struct member and an additional property that is used to support variable sized values.

### Leak-a-palooza
Currently, the API doesn't free memory allocated in the DLL.

While the DLL allocates memory using the appropriate CoAlloc method, the winstruct code does not free this memory... I guess I should address that if this project goes any further.
