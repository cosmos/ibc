package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Arg string

type Value string

type Test struct {
	Name     string `json:"name"`
	Function string `json:"function"`
	Args     []Arg  `json:"args"`
	Result   Value  `json:"result"`
}

func run(t Test) error {
	fmt.Printf("Ran test %s\n", t.Name)
	return nil
}

func main() {
	fh, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	bytes, err := ioutil.ReadAll(fh)
	if err != nil {
		panic(err)
	}
	var tests []Test
	err = json.Unmarshal(bytes, &tests)
	if err != nil {
		panic(err)
	}
	for _, test := range tests {
		err = run(test)
		if err != nil {
			panic(err)
		}
	}
}
