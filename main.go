package main

import (
	"fmt"
	"log"

	"sysadmin-go-education/checker"
)

// person, Go'nun temel türleriyle küçük bir struct gösterir.
type person struct {
	name string
	age  int
}

func main() {
	fmt.Println("✅ Go dilinin temelleri")
	fmt.Println("✅ Fonksiyonlar, paket yapısı ve modüller")

	u := person{name: "Ada", age: 40}
	fmt.Println("Merhaba,", u.name, "- yaş:", u.age)

	if err := checker.CheckSomething(u.name); err != nil {
		log.Println("kontrol sırasında hata:", err)
		return
	}

	log.Println("checker paketi her şey yolunda dedi")
}
