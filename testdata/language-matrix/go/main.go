package main

import "fmt"

func message(name string) string {
	return "hello, " + name
}

func main() {
	fmt.Println(message("buildworld"))
}
