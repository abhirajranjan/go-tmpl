# go templ

## Usage

first generate templ go code by using templ generate then pass the code along with args.
args are optional and the program would work as expected.

```bash
templ generate test/hello.go
go run main.go --templ test/hello_templ.go --data test/test.json
```

## JSON structure

json should only contain array of string

```json
["a", "1"]
["a", 1] // or
```
