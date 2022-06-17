package utils

func IsValidLuhn(number int64) bool {
	return (number%10+checksum(number/10))%10 == 0
}

func checksum(number int64) int64 {
	var luhn int64

	for i := 0; number > 0; i++ {
		digit := number % 10

		if i%2 == 0 {
			digit = digit * 2
			if digit > 9 {
				digit = digit%10 + digit/10
			}
		}

		luhn += digit
		number = number / 10
	}
	return luhn % 10
}
