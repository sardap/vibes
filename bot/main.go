package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	"github.com/sardap/discgov"
	"github.com/sardap/discom"
	"github.com/sardap/vibes/bot/vibes"
	bolt "go.etcd.io/bbolt"
)

const (
	prefix           = "-vb"
	starVibePattern  = "start"
	setupVibePattern = "setup ([a-z]{2}) \"(.*?)\" (-?\\d{4})"
)

var (
	dbClient     *bolt.DB
	commandSet   *discom.CommandSet
	starVibeRe   = regexp.MustCompile(starVibePattern)
	setupVibeRe  = regexp.MustCompile(setupVibePattern)
	vibesInvoker vibes.Invoker
	soundsPath   = os.Getenv("SOUNDS_PATH")
	bucketName   = []byte("guilds")
)

func init() {
	commandSet = discom.CreateCommandSet(regexp.MustCompile(prefix))

	err := commandSet.AddCommand(discom.Command{
		Re: starVibeRe, Handler: startVibeCmd,
		Example:     "start vibe",
		Description: "Joins the chat channel and vibes",
		CaseInSense: true,
	})
	if err != nil {
		panic(err)
	}

	err = commandSet.AddCommand(discom.Command{
		Re: setupVibeRe, Handler: setupVibeCmd,
		Example:     "setup US \"new york\" -0500",
		Description: "setup server info in bot db the number is the offset",
		CaseInSense: true,
	})
	if err != nil {
		panic(err)
	}

	vibesInvoker = vibes.Invoker{
		Endpoint:  os.Getenv("VIBES_ENDPOINT"),
		AccessKey: os.Getenv("VIBES_ACCESS_KEY"),
	}

	dbClient, err = bolt.Open(os.Getenv("DB_PATH"), 0666, nil)
	if err != nil {
		panic(err)
	}

	err = dbClient.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
}

type guildInfo struct {
	Country string `json:"country"`
	City    string `json:"city"`
	Offset  string `json:"offset"`
}

func getGuildInfo(id string) *guildInfo {
	var result *guildInfo
	dbClient.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(bucketName).Get([]byte(id))
		if val == nil {
			result = nil
			return nil
		}

		var g guildInfo
		json.Unmarshal(val, &g)
		result = &g

		return nil
	})

	return result
}

func setGuildInfo(id string, info guildInfo) error {
	dbClient.Update(func(tx *bolt.Tx) error {
		b, _ := json.Marshal(info)
		return tx.Bucket(bucketName).Put(
			[]byte(id), []byte(b),
		)
	})

	return nil
}

func getUserChannel(guildID, userID string, channels []*discordgo.Channel) (string, error) {
	for _, channel := range channels {
		users := discgov.GetUsers(guildID, channel.ID)
		for _, userID := range users {
			if userID == userID {
				return channel.ID, nil
			}
		}
	}

	return "", errors.New("Could not find user")
}

func joinCaller(
	s *discordgo.Session, m *discordgo.MessageCreate,
) (voice *discordgo.VoiceConnection, err error) {
	guild, err := s.State.Guild(m.GuildID)
	if err != nil {
		return nil, fmt.Errorf("could not find your discord server")
	}

	targetChannel, err := getUserChannel(m.GuildID, m.Author.ID, guild.Channels)
	if err != nil {
		return nil, fmt.Errorf("Must be in a channel on the target server to pick it up")
	}

	return s.ChannelVoiceJoin(m.GuildID, targetChannel, false, true)
}

func offsetTime(offset string) time.Time {
	offsetHour, _ := strconv.Atoi(offset[:2])
	offsetMin, _ := strconv.Atoi(offset[1:])
	if offsetHour < 0 {
		offsetMin = -offsetMin
	}

	return time.Now().UTC().Add(
		time.Duration(offsetHour+1) * time.Hour,
	).Add(
		time.Duration(offsetMin) * time.Minute,
	)
}

func (i *guildInfo) startVibing(
	invoker vibes.Invoker, v *discordgo.VoiceConnection,
	g *discordgo.Guild,
) {
	sets, err := invoker.GetSets()
	if err != nil {
		return
	}

	for {
		err := func() error {
			bytes, err := invoker.GetSample(
				offsetTime(i.Offset).Hour(), sets[rand.Intn(len(sets))], i.City, i.Country,
			)
			if err != nil {
				return err
			}
			fileName := filepath.Join(soundsPath, fmt.Sprintf("%d.ogg", rand.Int()))
			ioutil.WriteFile(fileName, bytes, 0644)
			defer os.Remove(filepath.Join(soundsPath, fileName))

			var startTime int
			offsetLeft := offsetTime(i.Offset).Minute() % 10
			if offsetLeft > 5 {
				offsetLeft = offsetLeft - 5
			}
			startTime = offsetLeft*60 + offsetTime(i.Offset).Second()

			options := dca.StdEncodeOptions
			options.RawOutput = true
			options.Bitrate = 100
			options.Volume = 50
			options.Application = "audio"
			options.StartTime = startTime
			encodingSession, err := dca.EncodeFile(fileName, options)
			if err != nil {
				return err
			}

			v.Speaking(true)
			defer v.Speaking(false)
			done := make(chan error)
			dca.NewStream(encodingSession, v, done)
			err = <-done
			// if err != nil && err.Error() != "EOF" {
			// 	return err
			// }
			encodingSession.Cleanup()

			return nil
		}()
		if err != nil {
			fmt.Printf("%v", err)
			return
		}
	}
}

func setupVibeCmd(s *discordgo.Session, m *discordgo.MessageCreate) {
	matches := setupVibeRe.FindAllStringSubmatch(strings.ToLower(m.Content), -1)
	country := matches[0][1]
	city := matches[0][2]
	timeOffsetStr := matches[0][3]

	offsetHour, err := strconv.Atoi(timeOffsetStr[:2])
	if err != nil || offsetHour > 14 || offsetHour < -12 {
		s.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf(
				"<@%s> invalid timezone offset",
				m.Author.ID,
			),
		)
	}

	setGuildInfo(m.GuildID, guildInfo{
		Country: country, City: city, Offset: timeOffsetStr,
	})

	s.ChannelMessageSend(
		m.ChannelID,
		fmt.Sprintf(
			"<@%s> created server info in DB!",
			m.Author.ID,
		),
	)
}

func startVibeCmd(s *discordgo.Session, m *discordgo.MessageCreate) {
	info := getGuildInfo(m.GuildID)
	if info == nil {
		s.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf(
				"<@%s> please setup server info first check help",
				m.Author.ID,
			),
		)
		return
	}

	v, err := joinCaller(s, m)
	if err != nil {
		s.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf(
				"<@%s> Unable to find you in a channel! Err: %v",
				m.Author.ID, err,
			),
		)
		return
	}

	g, _ := s.Guild(m.GuildID)

	info.startVibing(vibesInvoker, v, g)
}

func voiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	discgov.UserVoiceTrackerHandler(s, v)

	if len(discgov.GetUsers(v.GuildID, v.ChannelID)) == 0 {
		s.ChannelVoiceJoin(v.GuildID, "", true, true)
	}
}

func main() {
	token := strings.Replace(os.Getenv("DISCORD_AUTH"), "\"", "", -1)
	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("unable to create new discord instance")
		log.Fatal(err)
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	discord.AddHandler(commandSet.Handler)
	discord.AddHandler(voiceStateUpdate)

	// Open a websocket connection to Discord and begin listening.
	err = discord.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	discord.UpdateStatus(-1, "\"-vb help\"")

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	discord.Close()

}
