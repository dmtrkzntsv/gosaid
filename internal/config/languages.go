package config

// Subset of ISO 639-1 codes. Extend as needed — Whisper supports ~99 languages
// but most users will touch a handful. Keys are lowercase codes.
var languageNames = map[string]string{
	"ar": "Arabic",
	"bg": "Bulgarian",
	"cs": "Czech",
	"da": "Danish",
	"de": "German",
	"el": "Greek",
	"en": "English",
	"es": "Spanish",
	"et": "Estonian",
	"fa": "Persian",
	"fi": "Finnish",
	"fr": "French",
	"he": "Hebrew",
	"hi": "Hindi",
	"hr": "Croatian",
	"hu": "Hungarian",
	"id": "Indonesian",
	"it": "Italian",
	"ja": "Japanese",
	"ko": "Korean",
	"lt": "Lithuanian",
	"lv": "Latvian",
	"nl": "Dutch",
	"no": "Norwegian",
	"pl": "Polish",
	"pt": "Portuguese",
	"ro": "Romanian",
	"ru": "Russian",
	"sk": "Slovak",
	"sl": "Slovenian",
	"sr": "Serbian",
	"sv": "Swedish",
	"th": "Thai",
	"tr": "Turkish",
	"uk": "Ukrainian",
	"vi": "Vietnamese",
	"zh": "Chinese",
}

func IsValidLanguage(code string) bool {
	_, ok := languageNames[code]
	return ok
}

func LanguageName(code string) string {
	if n, ok := languageNames[code]; ok {
		return n
	}
	return code
}
