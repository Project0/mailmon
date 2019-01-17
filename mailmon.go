package mailmon

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/smtp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/emersion/go-dkim"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type (
	// MailMon is..
	MailMon struct {
		Log       *log.Logger
		TestSuite *TestSuite
		Client    *client.Client
	}

	TestSuite struct {
		Sender     string
		Recipient  string
		SmtpServer *SmtpConnection
		ImapServer *ImapConnection
	}
	SmtpConnection struct {
		Address  string
		Username string
		Password string
	}
	ImapConnection struct {
		Address  string
		Username string
		Password string
	}
)

const (
	// HeaderTrace is the mail header to identify the test mail
	HeaderTrace = "X-MailMon-Trace-ID"
)

// New test
func New(logger *log.Logger, suite *TestSuite) (*MailMon, error) {

	destClient, err := client.DialTLS(suite.ImapServer.addr(), nil)
	if err != nil {
		return nil, err
	}
	err = destClient.Login(suite.ImapServer.Username, suite.ImapServer.Password)
	if err != nil {
		return nil, err
	}
	destClient.SetDebug(log.NewEntry(logger).WriterLevel(log.TraceLevel))

	return &MailMon{
		TestSuite: suite,
		Log:       logger,
		Client:    destClient,
	}, nil
}

// WaitForMail waits until the mail appears in the inbox
func (m *MailMon) WaitForMail(id string) {

	m.Log.Infof("Wait for message with id %s", id)
	// Select INBOX
	_, err := m.Client.Select("INBOX", false)
	if err != nil {
		m.Log.Fatal(err)
	}

	seqFound := new(imap.SeqSet)

	for seqFound.Empty() {
		// takes some time until it appears at the destination server
		m.Log.Debug("Wait for message")
		time.Sleep(time.Second)

		// search for the mail
		sc := imap.NewSearchCriteria()
		sc.Header.Set(HeaderTrace, id)
		uids, err := m.Client.UidSearch(sc)
		if err != nil {
			m.Log.Fatal(err)
		}

		if len(uids) == 0 {
			continue
		}

		// Get the whole message body
		section := &imap.BodySectionName{}

		items := []imap.FetchItem{section.FetchItem(), imap.FetchFlags}
		messages := make(chan *imap.Message, 1)

		seqset := new(imap.SeqSet)
		seqset.AddNum(uids...)

		go func() {
			err = m.Client.UidFetch(seqset, items, messages)
			if err != nil {
				m.Log.Fatal(err)
			}
		}()

		for msg := range messages {
			if msg == nil {
				m.Log.Fatal("Server didn't returned message")
			}
			seqFound.AddNum(msg.Uid)

			// read body for multiple usecases
			body, err := ioutil.ReadAll(msg.GetBody(section))
			if err != nil {
				m.Log.Error(err)
				continue
			}

			// encodedmsg, err := message.Read(bytes.NewBuffer(body))
			if err != nil {
				m.Log.Error(err)
			} else {
				// check dkim signature
				ver, err := dkim.Verify(bytes.NewBuffer(body))
				if err != nil {
					m.Log.Error(err)
				} else {
					for _, v := range ver {
						if v.Err == nil {
							m.Log.Infof("Valid dkim signature for %s", v.Domain)
						} else {
							m.Log.Errorf("Invalid dkim signature for %s: %v", v.Domain, v.Err)
						}
					}
				}
			}

		}
	}
	if !seqFound.Empty() {
		m.delete(seqFound)
	}
}

// delete purges mails
func (m *MailMon) delete(seqset *imap.SeqSet) error {
	m.Log.Debug("Cleanup messages with the given id")
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}

	err := m.Client.UidStore(seqset, item, flags, nil)
	if err != nil {
		return err
	}
	return m.Client.Expunge(nil)
}

// Close shutdowns all connections
func (m *MailMon) Close() {
	if m.Client != nil {
		m.Client.Logout()
	}
}

// SendMessage sends the test message through the given server
func (m *MailMon) SendMessage(id string) error {
	msg := newMessage(id, m.TestSuite.Recipient, m.TestSuite.Sender, "MailMon test mail", "This is a test mail by MailMon")
	m.Log.Infof("Send message with id %s", id)
	return smtp.SendMail(m.TestSuite.SmtpServer.addr(), m.TestSuite.SmtpServer.auth(), m.TestSuite.Sender, []string{m.TestSuite.Recipient}, msg)
}

func (s *SmtpConnection) auth() smtp.Auth {
	if len(s.Username) == 0 {
		// no auth
		return nil
	}
	return smtp.PlainAuth("", s.Username, s.Password, "mail.starkiller.project0.de")
}

func (s *SmtpConnection) addr() string {
	if strings.Contains(s.Address, ":") {
		return s.Address
	}
	return fmt.Sprintf("%s:%d", s.Address, 587)
}

func (s *ImapConnection) addr() string {
	if strings.Contains(s.Address, ":") {
		return s.Address
	}
	return fmt.Sprintf("%s:%d", s.Address, 993)
}

func newMessage(id, to, from, subject, message string) []byte {
	buf := bytes.NewBuffer(make([]byte, 0))
	// header
	fmt.Fprintf(buf, "To: %s\r\n", to)
	fmt.Fprintf(buf, "From: %s\r\n", from)
	fmt.Fprintf(buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(buf, "%s: %s\r\n", HeaderTrace, id)
	// body
	buf.WriteString("\r\n")
	buf.WriteString(strings.Replace(message, "\n", "\r\n", -1))
	return buf.Bytes()
}
