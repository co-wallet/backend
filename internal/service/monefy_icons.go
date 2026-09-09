package service

import (
	"math/rand/v2"
	"slices"
	"strings"
	"unicode"
)

// Keep these IDs aligned with AccountIcon and CategoryIcon in the frontend.
var importAccountPresets = strings.Fields("cash ruble dollar euro bank debit-card credit-card coins piggy-bank wallet investments bitcoin savings shared car travel")
var importCategoryPresets = strings.Fields("other groceries fast-food pizza cafe drinks restaurants car fuel bus travel taxi train home utilities tax percent mobile phone subscriptions electronics repairs cleaning washing household-chemicals clothes shoes beauty shopping accessories watch medicine health medical psychology first-aid care dentistry glasses movies games music books fitness sport running campfire smoking pets garden gifts services household delivery parents family people debt-return baby baby-carriage heart armchair couch fish bowl-food chef oven cooking-pot beer-stein wine cocktail ice-cream film-reel camera smiley guitar music-notes book-open t-shirt shirt-folded coat-hanger high-heel flower thermometer horse cat dog boat globe island motorcycle moped bicycle police tire basketball tennis football skiing paint-brush paint-roller wrench screwdriver toolbox storefront calculator cpu lightning work savings cash card investments bank partnership education salary business coins debts cashback interest rent startup bonus events gift-income award")

var importIconColors = []string{"blue", "purple", "pink", "red", "orange", "green", "yellow", "graphite"}

func validImportIcon(value string, presets []string) bool {
	parts := strings.Split(value, "|")
	if !strings.HasPrefix(parts[0], "preset:") || !slices.Contains(presets, strings.TrimPrefix(parts[0], "preset:")) {
		return false
	}
	return len(parts) == 1 || (len(parts) == 3 && slices.Contains(importIconColors, parts[1]) && (parts[2] == "none" || slices.Contains(importIconColors, parts[2])))
}

type importIconRule struct{ preset, aliases string }

// Match complete words/phrases, avoiding accidental substring matches (e.g. карта/картография).
// Rule order gives specific phrases priority over broader words. All data stays on this server.
var importAccountRules = []importIconRule{
	{"credit-card", "кредитная карта|кредитка|credit card"},
	{"cash", "наличные|наличка|нал|кеш|кэш|cash"},
	{"debit-card", "карта|карточка|дебетовая|card|debit"},
	{"wallet", "кошелек|кошелёк|wallet"},
	{"savings", "вклад|депозит|накопления|сбережения|deposit|savings"},
	{"piggy-bank", "копилка|piggy bank"},
	{"investments", "инвестиции|брокерский|акции|investments|stocks"},
	{"bitcoin", "биткоин|bitcoin|btc"},
	{"bank", "банк|bank"},
	{"travel", "отпуск|путешествия|travel|vacation"},
}
var importExpenseRules = []importIconRule{
	{"cafe", "кафе|кофе|кофейня|кофейни|coffee|cafe|café"},
	{"restaurants", "ресторан|рестораны|столовая|restaurants|restaurant|dining"},
	{"groceries", "продукты|продуктовый|еда|питание|супермаркет|супермаркеты|food|groceries|grocery|supermarket"},
	{"taxi", "такси|taxi|uber"},
	{"fuel", "бензин|топливо|азс|fuel|gasoline"},
	{"bus", "транспорт|проезд|автобус|метро|transport|transportation|bus|subway"},
	{"car", "авто|автомобиль|машина|car"},
	{"home", "дом|жилье|жильё|квартира|home|housing"},
	{"utilities", "коммунальные|коммуналка|жкх|utilities"},
	{"medicine", "лекарства|аптека|pharmacy|medicine"},
	{"health", "здоровье|лечение|health|healthcare"},
	{"clothes", "одежда|clothes|clothing"},
	{"travel", "отпуск|путешествия|travel|vacation"},
	{"gifts", "подарок|подарки|gift|gifts"},
	{"pets", "питомцы|животные|pets"},
	{"sport", "спорт|фитнес|sport|fitness"},
	{"mobile", "связь|телефон|mobile|phone"},
	{"subscriptions", "подписки|подписка|subscription|subscriptions"},
}
var importIncomeRules = []importIconRule{
	{"salary", "зарплата|заработная плата|зп|з п|salary|wage|wages|payroll|paycheck"},
	{"bonus", "премия|бонус|bonus"},
	{"cashback", "кешбэк|кэшбэк|кешбек|кэшбек|cashback"},
	{"interest", "проценты|процент|interest"},
	{"rent", "аренда|rent"},
	{"gift-income", "подарок|подарки|gift|gifts"},
	{"investments", "инвестиции|дивиденды|investments|dividends"},
	{"work", "работа|подработка|фриланс|work|freelance"},
	{"cash", "наличные|наличка|cash"},
	{"card", "карта|карточка|card"},
}

func suggestImportIcon(name, kind string) string {
	normalized := " " + strings.Join(strings.FieldsFunc(strings.ReplaceAll(strings.ToLower(name), "ё", "е"), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ") + " "
	preset := "other"
	rules := importExpenseRules
	switch kind {
	case "account":
		preset, rules = "wallet", importAccountRules
	case "income":
		rules = importIncomeRules
	}
	for _, rule := range rules {
		matched := false
		for _, alias := range strings.Split(rule.aliases, "|") {
			if strings.Contains(normalized, " "+strings.ReplaceAll(alias, "ё", "е")+" ") {
				matched = true
				break
			}
		}
		if matched {
			preset = rule.preset
			break
		}
	}
	// Appearance is generated only when absent, then persisted in the immutable snapshot.
	color := importIconColors[rand.IntN(len(importIconColors))]
	return "preset:" + preset + "|" + color + "|" + color
}
