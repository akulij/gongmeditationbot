package main

import (
	"context"
	"fmt"
	"log"
    "time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func (bc *BotController) SyncPaidUsersToSheet() error {
    reservations, _ := bc.GetAllReservations()

	ctx := context.Background()
	srv, err := sheets.NewService(ctx,
        option.WithCredentialsFile("./credentials.json"),
        option.WithScopes(sheets.SpreadsheetsScope),
    )
	if err != nil {
		return fmt.Errorf("unable to retrieve Sheets client: %v", err)
	}

	var values [][]interface{}
    values = append(values, []interface{}{"Телеграм ID", "Имя", "Фамилия", "Никнейм", "Указанное имя", "Дата", "Телефон", "Статус"})

	for _, reservation := range reservations {
        if reservation.Status != Paid {continue}

        uid := reservation.UserID
        user, _ := bc.GetUserByID(uid)
        ui, _ := bc.GetUserInfo(uid)
        event, _ := bc.GetEvent(reservation.EventID)
        status := ReservationStatusString[reservation.Status]

		values = append(values, []interface{}{user.ID, ui.FirstName, ui.LastName, ui.Username, "TODO", formatDate(event.Date), "", status})
	}

	// Prepare the data to be written to the sheet
	valueRange := &sheets.ValueRange{
		Values: values,
	}

	_, err = srv.Spreadsheets.Values.Clear(bc.cfg.SheetID, "A1:Z1000", &sheets.ClearValuesRequest{}).Do()

	// Write the data to the specified range in the sheet
	_, err = srv.Spreadsheets.Values.Update(bc.cfg.SheetID, "A1", valueRange).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("unable to write data to sheet: %v", err)
	}

	log.Printf("Successfully synced %d reservations to the Google Sheet.", len(reservations))
	return nil
}

func formatDate(t *time.Time) string {
    return t.Format("02.01 15:04")
}
