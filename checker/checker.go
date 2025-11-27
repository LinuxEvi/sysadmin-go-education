package checker

import (
	"errors"
	"fmt"
)

// CheckSomething temel bir validasyon örneği sunar.
func CheckSomething(input string) error {
	if input == "" {
		return errors.New("boş isim kontrolü geçemedi")
	}
	fmt.Println("checker paketi ismı doğruladı:", input)
	return nil
}
