package main

import (
	"fmt"
	"os"

	"github.com/project0/mailmon"

	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
	cli "gopkg.in/urfave/cli.v1" // imports as package "cli"
)

func checkRequiredString(c *cli.Context, args ...string) error {
	for _, a := range args {
		if len(c.String(a)) == 0 {
			return fmt.Errorf("flag is required: %s", a)
		}
	}
	return nil
}

func main() {
	logger := log.New()

	app := cli.NewApp()
	app.Name = "MailMon"
	app.Usage = "An easy way to test mail communication"
	app.HideVersion = true

	globalFlags := []cli.Flag{
		&cli.IntFlag{
			Name:  "log, l",
			Value: int(log.InfoLevel),
			Usage: "Log level, from 0 (panic) to 6 (trace)  (Global)",
		},
	}
	beforeGlobal := func(c *cli.Context) error {
		level := c.Int("log")
		if level > len(log.AllLevels)-1 {
			return fmt.Errorf("Invalid log level")
		}
		logger.SetLevel(log.AllLevels[level])
		return nil
	}

	testCmd := cli.Command{
		Name:      "test",
		Usage:     "Test mail delivery to an IMAP server",
		UsageText: "This test submits an mail through the given smtp address and checks if it has been delivered to the imap server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "from",
				Usage: "Envelope sender address",
			},
			&cli.StringFlag{
				Name:  "to",
				Usage: "Envelope recipient address",
			},
			&cli.StringFlag{
				Name:  "smtp-address, s",
				Usage: "SMTP (relay) server adress to submit",
			},
			&cli.StringFlag{
				Name:  "smtp-username, su",
				Usage: "SMTP (relay) login username",
			},
			&cli.StringFlag{
				Name:  "smtp-password, sp",
				Usage: "SMTP (relay) login password",
			},
			&cli.StringFlag{
				Name:  "imap-address, i",
				Usage: "Target IMAP server adress to submit",
			},
			&cli.StringFlag{
				Name:  "imap-username, iu",
				Usage: "Target IMAP login username",
			},
			&cli.StringFlag{
				Name:  "imap-password, ip",
				Usage: "Target IMAP login password",
			},
			/*			&cli.DurationFlag{
						Name:  "timeout,t",
						Usage: "Timeout",
						Value: time.Minute,
					},*/
			&cli.StringFlag{
				Name:  "id",
				Value: "",
				Usage: "Set optional custom trace ID",
			},
			&cli.BoolFlag{
				Name:  "no-smtp",
				Usage: "Disable smtp delivery and check only destination server",
			},
		},
		Action: func(c *cli.Context) error {
			var (
				smtp *mailmon.SmtpConnection
				imap *mailmon.ImapConnection
			)

			if err := checkRequiredString(c, "from", "to", "imap-address", "imap-username"); err != nil {
				return err
			}

			imap = &mailmon.ImapConnection{
				Address:  c.String("imap-address"),
				Username: c.String("imap-username"),
				Password: c.String("imap-password"),
			}

			if !c.Bool("no-smtp") {
				if err := checkRequiredString(c, "smtp-address"); err != nil {
					return err
				}
				smtp = &mailmon.SmtpConnection{
					Address:  c.String("smtp-address"),
					Username: c.String("smtp-username"),
					Password: c.String("smtp-password"),
				}
			}

			id := c.String("id")
			if len(id) == 0 {
				newID, err := uuid.NewUUID()
				if err != nil {
					return err
				}
				id = newID.String()
			}

			suite := &mailmon.TestSuite{
				Sender:     c.String("from"),
				Recipient:  c.String("to"),
				SmtpServer: smtp,
				ImapServer: imap,
			}
			mon, err := mailmon.New(logger, suite)
			if err != nil {
				return err
			}
			if smtp != nil {
				if err := mon.SendMessage(id); err != nil {
					return err
				}
			}

			mon.WaitForMail(id)
			return nil
		},
	}

	testCmd.Flags = append(testCmd.Flags, globalFlags...)
	testCmd.Before = beforeGlobal
	app.Commands = []cli.Command{testCmd}

	err := app.Run(os.Args)
	if err != nil {
		logger.Fatal(err)
	}

}
