package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/sardap/discgov"
	"github.com/sardap/discom"
	"github.com/sardap/vibes/bot/vibes"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/semaphore"
)

const (
	starVibePattern   = "start"
	setupVibePattern  = "setup ([a-z]{2}) \"(.*?)\" (-?\\d{4})"
	stopVibePattern   = "stop"
	serverInfoPattern = "info"
)

var (
	prefix         = os.Getenv("BOT_PREFIX")
	dbClient       *bolt.DB
	commandSet     *discom.CommandSet
	starVibeRe     = regexp.MustCompile(starVibePattern)
	setupVibeRe    = regexp.MustCompile(setupVibePattern)
	stopVibeRe     = regexp.MustCompile(stopVibePattern)
	serverInfoRe   = regexp.MustCompile(serverInfoPattern)
	vibesInvoker   vibes.Invoker
	soundsPath     = os.Getenv("SOUNDS_PATH")
	bucketName     = []byte("guilds")
	voiceLocks     = cmap.New()
	defaultOptions = dca.StdEncodeOptions
)

func init() {
	defaultOptions.RawOutput = true
	defaultOptions.Volume = 50
	defaultOptions.Application = "audio"

	commandSet = discom.CreateCommandSet(regexp.MustCompile(prefix))

	err := commandSet.AddCommand(discom.Command{
		Re: starVibeRe, Handler: startVibeCmd,
		Example:     "start",
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

	err = commandSet.AddCommand(discom.Command{
		Re: stopVibeRe, Handler: stopVibeCmd,
		Example:     "stop",
		Description: "stop vibing",
		CaseInSense: true,
	})
	if err != nil {
		panic(err)
	}

	err = commandSet.AddCommand(discom.Command{
		Re: serverInfoRe, Handler: serverInfoCmd,
		Example:     "info",
		Description: "get server setup info",
		CaseInSense: true,
	})
	if err != nil {
		panic(err)
	}

	vibesInvoker = vibes.Invoker{
		Endpoint:  os.Getenv("VIBES_ENDPOINT"),
		AccessKey: os.Getenv("VIBES_ACCESS_KEY"),
		Scheme:    os.Getenv("VIBES_SCHEME"),
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

type voiceLock struct {
	lock    *semaphore.Weighted
	channel string
	playing string
}

func getVoiceLock(gid string) *voiceLock {
	var result *voiceLock
	if tmp, ok := voiceLocks.Get(gid); ok {
		result = tmp.(*voiceLock)
	}

	return result
}

func createVoiceLock(gid, cid string) *voiceLock {
	result := &voiceLock{
		lock:    semaphore.NewWeighted(1),
		channel: cid,
	}
	voiceLocks.Set(gid, result)
	return result
}

func deleteVoiceLock(gid string) {
	voiceLocks.Remove(gid)
}

func inVoice(gid string) bool {
	l := getVoiceLock(gid)

	if l == nil {
		return false
	}

	if l.lock.TryAcquire(1) {
		l.lock.Release(1)
		return false
	}

	return true
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
		return nil, fmt.Errorf("Must be in a channel on the target server to vibe")
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

//Gross
func firstDigit(x int) int {
	if x < 10 {
		return 0
	}
	str := strconv.Itoa(x)
	result, _ := strconv.Atoi(string(str[0]))
	return result
}

func createSeed(offset string) int64 {
	t := offsetTime(offset)
	str := fmt.Sprintf(
		"%d%d%d%d%d",
		firstDigit(t.Minute()), t.Hour(), t.Day(), t.Month(), t.Year(),
	)

	result, _ := strconv.ParseInt(str, 10, 64)
	fmt.Printf("seed: %s, int: %d\n", str, result)
	return result
}

func randomGame(sets []string, offset string) string {
	rand.Seed(createSeed(offset))
	return sets[rand.Intn(len(sets))]
}

func (i *guildInfo) startVibing(
	invoker vibes.Invoker, v *discordgo.VoiceConnection,
	g *discordgo.Guild,
) {
	sets, err := invoker.GetSets()
	if err != nil {
		return
	}

	vl := createVoiceLock(v.GuildID, v.ChannelID)
	vl.lock.Acquire(context.TODO(), 1)
	defer vl.lock.Release(1)
	defer deleteVoiceLock(v.GuildID)

	bellPlayed := false
	lastHour := -1
	for {
		//Check if it's the next hour
		if lastHour != offsetTime(i.Offset).Hour() {
			bellPlayed = false
			lastHour = offsetTime(i.Offset).Hour()
		}
		err := func() error {
			vl := getVoiceLock(v.GuildID)
			if vl == nil {
				return fmt.Errorf("disconnected")
			}

			offsetStart := false
			if !bellPlayed && firstDigit(offsetTime(i.Offset).Minute()) == 0 {
				fmt.Printf("BELL TIME\n")
				stream, err := invoker.GetBellStream()
				if err != nil {
					return err
				}
				defer stream.Close()
				bellPlayed = true
				encodingSession, err := dca.EncodeMem(stream, defaultOptions)
				if err != nil {
					return err
				}

				v.Speaking(true)
				done := make(chan error)
				dca.NewStream(encodingSession, v, done)
				err = <-done
				v.Speaking(false)
				encodingSession.Cleanup()
			}

			stream, err := invoker.GetSampleStream(
				offsetTime(i.Offset).Hour(), randomGame(sets, i.Offset), i.City, i.Country,
			)
			defer stream.Close()
			offsetStart = true
			if err != nil {
				return err
			}

			options := defaultOptions
			if offsetStart {
				var startTime int
				offsetLeft := offsetTime(i.Offset).Minute() % 10
				startTime = offsetLeft*60 + offsetTime(i.Offset).Second()
				options.StartTime = startTime
			}

			encodingSession, err := dca.EncodeMem(stream, options)
			if err != nil {
				return err
			}

			v.Speaking(true)
			defer v.Speaking(false)
			done := make(chan error)
			dca.NewStream(encodingSession, v, done)
			err = <-done
			encodingSession.Cleanup()

			return nil
		}()
		if err != nil {
			fmt.Printf("%v\n", err)
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

func serverInfoCmd(s *discordgo.Session, m *discordgo.MessageCreate) {
	info := getGuildInfo(m.GuildID)
	if info == nil {
		s.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf(
				"<@%s> No sever info in my DB!",
				m.Author.ID,
			),
		)
		return
	}

	s.ChannelMessageSend(
		m.ChannelID,
		fmt.Sprintf(
			"<@%s> info %v\n hour: %d",
			m.Author.ID, info, offsetTime(info.Offset).Hour(),
		),
	)

}

func startVibeCmd(s *discordgo.Session, m *discordgo.MessageCreate) {
	if inVoice(m.GuildID) {
		s.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf(
				"<@%s> already vibing dude",
				m.Author.ID,
			),
		)
		return
	}

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

	go info.startVibing(vibesInvoker, v, g)
}

func stopVibeCmd(s *discordgo.Session, m *discordgo.MessageCreate) {
	if !inVoice(m.GuildID) {
		s.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf(
				"<@%s> no vibes are happening right now",
				m.Author.ID,
			),
		)
	}

	deleteVoiceLock(m.GuildID)
	s.ChannelVoiceJoin(m.GuildID, "", true, true)
}

func voiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	discgov.UserVoiceTrackerHandler(s, v)

	if usr, _ := s.User(v.UserID); usr.Bot {
		return
	}

	gvi := getVoiceLock(v.GuildID)
	if gvi == nil {
		return
	}
	if v.UserID == s.State.User.ID {
		if gvi.channel != v.ChannelID {
			gvi.channel = v.ChannelID
		}
		return
	}

	if len(discgov.GetUsers(v.GuildID, gvi.channel)) == 0 {
		s.ChannelVoiceJoin(v.GuildID, "", true, true)
		deleteVoiceLock(v.GuildID)
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

	discord.UpdateStatus(-1, fmt.Sprintf("\"%s help\"", prefix))

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	discord.Close()

}
