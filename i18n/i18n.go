package i18n

import (
	"log"
	"os"
	"strings"

	"github.com/jeandeaual/go-locale"
)

var lang string

var translations = map[string]map[string]string{
	"mm:ss or seconds": {
		"pt": "mm:ss ou segundos",
		"es": "mm:ss o segundos",
		"ru": "мм:сс или секунды",
	},
	"Stop": {
		"pt": "Parar",
		"es": "Parar",
		"ru": "Стоп",
	},
	"Start": {
		"pt": "Iniciar",
		"es": "Iniciar",
		"ru": "Старт",
	},
	"Reset": {
		"pt": "Resetar",
		"es": "Reiniciar",
		"ru": "Сброс",
	},
	"About D2Timers": {
		"pt": "Sobre o D2Timers",
		"es": "Acerca de D2Timers",
		"ru": "О D2Timers",
	},
	"Close": {
		"pt": "Fechar",
		"es": "Cerrar",
		"ru": "Закрыть",
	},
	"Help": {
		"pt": "Ajuda",
		"es": "Ayuda",
		"ru": "Помощь",
	},
}

func init() {
	// Check for override environment variable
	if forcedLang := strings.TrimSpace(os.Getenv("D2TIMERS_LANG")); forcedLang != "" { // Force language for testing purposes
		log.Printf("D2TIMERS_LANG is set to: '%s'", forcedLang)
		lang = forcedLang
		return
	}

	log.Println("D2TIMERS_LANG is not set, detecting from system locale.")
	userLocales, err := locale.GetLocales()
	if err != nil {
		log.Println("Could not get user locale, defaulting to english")
		lang = "en"
		return
	}

	log.Printf("Raw user locales detected: %v", userLocales)

	if len(userLocales) > 0 {
		locale := strings.ToLower(userLocales[0])
		log.Printf("Processing primary user locale: %s", locale)
		if strings.Contains(locale, "pt") {
			lang = "pt"
		} else if strings.Contains(locale, "es") {
			lang = "es"
		} else if strings.Contains(locale, "ru") {
			lang = "ru"
		} else {
			lang = "en"
		}
	} else {
		log.Println("No user locale detected, defaulting to english")
		lang = "en"
	}
	log.Printf("Language set to: %s", lang)
}

func T(key string) string {
	if translated, ok := translations[key][lang]; ok {
		return translated
	}
	return key
}

func GetLang() string {
	return lang
}
