package main

import (
	"fmt"
	"log"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xuri/excelize/v2"
)

type Student struct {
	Room  string
	Date  string
	Name  string
	Birth string
	Phone string
	Work  string
}

type UserData struct {
	Step           int
	TempData       Student
	Students       []Student
	IsFilling      bool
	GlobalDate     string
	GlobalWork     string
	PendingConfirm string
	DateLocked     bool
	WorkLocked     bool
}

var users = make(map[int64]*UserData)

func main() {
	bot, err := tgbotapi.NewBotAPI("8400970685:AAEVOk4dnNYNYm6xFfKOFwA7qwD0Ut0sbVg")
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Бот запущен: %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	mainMenu := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Табло для Вани"),
		),
	)
	mainMenu.ResizeKeyboard = true

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, update.Message, mainMenu)
		}
		if update.CallbackQuery != nil {
			handleCallback(bot, update.CallbackQuery)
		}
	}
}

// --- клавиатура во время заполнения ---
func fillingKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Завершить ввод"),
		),
	)
}

// --- обработка сообщений ---
func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, mainMenu tgbotapi.ReplyKeyboardMarkup) {
	userID := msg.Chat.ID
	text := msg.Text

	switch text {
	case "/start":
		m := tgbotapi.NewMessage(userID, "Здравствуйте, Иван Анатольевич!\nВыберите действие:")
		m.ReplyMarkup = mainMenu
		bot.Send(m)
		return

	case "Табло для Вани":
		user, exists := users[userID]
		if exists && user.IsFilling {
			bot.Send(tgbotapi.NewMessage(userID, "Вы уже заполняете данные. Введите № Квартиры/дома или завершите ввод."))
			return
		}
		users[userID] = &UserData{Step: 1, IsFilling: true}
		msg := tgbotapi.NewMessage(userID, "Ваня, ешь яйца....\nВведите № Квартиры/дома:")
		msg.ReplyMarkup = fillingKeyboard()
		bot.Send(msg)
		return

	case "Завершить ввод", "/end":
		user, exists := users[userID]
		if !exists || len(user.Students) == 0 {
			bot.Send(tgbotapi.NewMessage(userID, "Тут нечего сохранять."))
			return
		}
		filePath := generateExcel(user.Students)
		sendExcel(bot, userID, filePath)
		delete(users, userID)
		msg := tgbotapi.NewMessage(userID, "Ввод завершён.")
		msg.ReplyMarkup = mainMenu
		bot.Send(msg)
		return
	}

	user, exists := users[userID]
	if !exists || !user.IsFilling {
		bot.Send(tgbotapi.NewMessage(userID, "Нажмите кнопку 'Табло для Вани', чтобы начать ввод."))
		return
	}

	if user.PendingConfirm != "" {
		bot.Send(tgbotapi.NewMessage(userID, "Ответьте сначала (Да или Нет)."))
		return
	}

	switch user.Step {
	case 1:
		user.TempData.Room = text
		if user.GlobalDate != "" {
			user.TempData.Date = user.GlobalDate
			user.Step = 3
			bot.Send(tgbotapi.NewMessage(userID, "ФИО:"))
		} else {
			user.Step = 2
			bot.Send(tgbotapi.NewMessage(userID, "Дата беседы:"))
		}

	case 2:
		user.TempData.Date = text
		if user.GlobalDate == "" && !user.DateLocked {
			user.PendingConfirm = "DATE"
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Да", "CONFIRM_DATE_YES"),
					tgbotapi.NewInlineKeyboardButtonData("Нет", "CONFIRM_DATE_NO"),
				),
			)
			m := tgbotapi.NewMessage(userID, "Применить эту дату ко всем записям?")
			m.ReplyMarkup = keyboard
			bot.Send(m)
		} else {
			user.Step = 3
			bot.Send(tgbotapi.NewMessage(userID, "ФИО:"))
		}

	case 3:
		user.TempData.Name = text
		user.Step = 4
		bot.Send(tgbotapi.NewMessage(userID, "Год рождения:"))

	case 4:
		user.TempData.Birth = text
		user.Step = 5
		bot.Send(tgbotapi.NewMessage(userID, "Номер телефона:"))

	case 5:
		user.TempData.Phone = text
		if user.GlobalWork != "" {
			user.TempData.Work = user.GlobalWork
			addStudentAndAskNext(bot, userID, user)
		} else {
			user.Step = 6
			bot.Send(tgbotapi.NewMessage(userID, "Место работы (учёбы):"))
		}

	case 6:
		user.TempData.Work = text
		if user.GlobalWork == "" && !user.WorkLocked {
			user.PendingConfirm = "WORK"
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Да", "CONFIRM_WORK_YES"),
					tgbotapi.NewInlineKeyboardButtonData("Нет", "CONFIRM_WORK_NO"),
				),
			)
			m := tgbotapi.NewMessage(userID, "Применить это место ко всем записям?")
			m.ReplyMarkup = keyboard
			bot.Send(m)
		} else {
			addStudentAndAskNext(bot, userID, user)
		}
	}
}

// --- добавление студента ---
func addStudentAndAskNext(bot *tgbotapi.BotAPI, userID int64, user *UserData) {
	user.Students = append(user.Students, user.TempData)
	user.TempData = Student{}
	user.Step = 1

	message := fmt.Sprintf("Данные добавлены (%d записей).\nЧтобы завершить ввод — нажмите 'Завершить ввод'.\n\nДобавлено:", len(user.Students))
	for i, s := range user.Students {
		message += fmt.Sprintf("\n%d. %s", i+1, s.Name)
	}
	message += "\n\nСледующий № Квартира/дома:"

	msg := tgbotapi.NewMessage(userID, message)
	msg.ReplyMarkup = fillingKeyboard()
	bot.Send(msg)
}

// --- обработка inline-кнопок ---
func handleCallback(bot *tgbotapi.BotAPI, cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	user, ok := users[chatID]
	if !ok {
		bot.Request(tgbotapi.NewCallback(cb.ID, ""))
		return
	}

	switch cb.Data {
	case "CONFIRM_DATE_YES":
		user.GlobalDate = user.TempData.Date
		user.PendingConfirm = ""
		user.Step = 3
		bot.Send(tgbotapi.NewMessage(chatID,
			fmt.Sprintf("Дата '%s' будет применяться ко всем новым записям.\nВведи ФИО:", user.GlobalDate),
		))

	case "CONFIRM_DATE_NO":
		user.PendingConfirm = ""
		user.DateLocked = true
		user.Step = 3
		bot.Send(tgbotapi.NewMessage(chatID, "Хорошо, дата будет вводиться вручную для каждой записи.\nВведи ФИО:"))

	case "CONFIRM_WORK_YES":
		user.GlobalWork = user.TempData.Work
		user.PendingConfirm = ""
		addStudentAndAskNext(bot, chatID, user)

	case "CONFIRM_WORK_NO":
		user.PendingConfirm = ""
		user.WorkLocked = true
		addStudentAndAskNext(bot, chatID, user)
	}

	bot.Request(tgbotapi.NewCallback(cb.ID, ""))
}

// --- генерация Excel с форматированием ---
func generateExcel(data []Student) string {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)

	headers := []string{"№ Квартиры/дома", "Дата беседы", "ФИО", "Год рождения", "Номер телефона", "Место работы (учёбы)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for rowIdx, s := range data {
		values := []string{s.Room, s.Date, s.Name, s.Birth, s.Phone, s.Work} // телефон как текст (с апострофом)
		for colIdx, val := range values {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, val)
		}
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 14, Family: "Times New Roman"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#B4B4B4"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	dataStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11, Family: "Calibri"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	lastRow := len(data) + 1
	f.SetCellStyle(sheet, "A1", "F1", headerStyle)
	if lastRow >= 2 {
		f.SetCellStyle(sheet, "A2", fmt.Sprintf("F%d", lastRow), dataStyle)
	}

	f.AutoFilter(sheet, fmt.Sprintf("A1:F%d", lastRow), []excelize.AutoFilterOptions{})

	widths := map[string]float64{
		"A": 25, "B": 25, "C": 50, "D": 25, "E": 30, "F": 45,
	}
	for col, w := range widths {
		f.SetColWidth(sheet, col, col, w)
	}
	f.SetRowHeight(sheet, 1, 30)

	fileName := fmt.Sprintf("ОЖС_%s.xlsx", time.Now().Format("02-01-2006"))
	_ = f.SaveAs(fileName)
	return fileName
}

// --- отправка файла ---
func sendExcel(bot *tgbotapi.BotAPI, chatID int64, filePath string) {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	doc.Caption = "Файл готов."
	bot.Send(doc)
	_ = os.Remove(filePath)
}
