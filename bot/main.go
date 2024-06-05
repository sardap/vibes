package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/sardap/discgov"
	"github.com/sardap/vibes/bot/vibes"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/semaphore"
)

const (
	setupVibePattern = "setup ([a-z]{2}) \"(.*?)\" (-?\\d{4})"
)

var (
	dbClient       *bolt.DB
	bucketName     = []byte("guilds")
	voiceLocks     = cmap.New()
	defaultOptions = dca.StdEncodeOptions
)

type vibeInfo struct {
	command string
	invoker vibes.Invoker
}

type commandSet struct {
	commands map[string]*discordgo.ApplicationCommand
	handlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func createCommandSet(s *discordgo.Session) commandSet {
	defaultOptions.RawOutput = true
	defaultOptions.Volume = 50
	defaultOptions.Application = "audio"

	commands := make(map[string]*discordgo.ApplicationCommand)

	commands["setup"] = &discordgo.ApplicationCommand{
		Name:        "setup",
		Description: "setup server info in bot db",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "country",
				Description: "US (Country Code)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "city",
				Description: "new york",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "time-offset",
				Description: "-0500",
				Required:    true,
			},
		},
	}

	commands["info"] = &discordgo.ApplicationCommand{
		Name:        "info",
		Description: "get guild info",
		Options:     []*discordgo.ApplicationCommandOption{},
	}

	commands["stop"] = &discordgo.ApplicationCommand{
		Name:        "stop",
		Description: "stops the vibes",
		Options:     []*discordgo.ApplicationCommandOption{},
	}

	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"setup": setupVibeCmd,
		"info":  guildInfoCmd,
		"stop":  stopVibeCmd,
	}

	var err error
	dbClient, err = bolt.Open(os.Getenv("DB_PATH"), 0666, nil)
	if err != nil {
		log.Fatal(err)
	}

	dbClient.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})

	vibeSets := make(map[string]*vibeInfo)
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0)

	i := 0
	vibesKeys := make([]string, 0)
	for {
		str := os.Getenv(fmt.Sprintf("VIBES_%d", i))
		if str == "" {
			break
		}

		splits := strings.Split(str, ",")
		if len(splits) != 4 {
			panic("vibe info has wrong number of args")
		}

		username := os.Getenv("VIBES_USERNAME")
		password := os.Getenv("VIBES_PASSWORD")

		vibesKeys = append(vibesKeys, splits[0])
		vibeSets[splits[0]] = &vibeInfo{
			command: splits[0],
			invoker: vibes.Invoker{
				Endpoint:  splits[2],
				AccessKey: splits[3],
				Scheme:    splits[1],
				Username:  username,
				Password:  password,
			},
		}

		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  splits[0],
			Value: splits[0],
		})

		i++
	}

	choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
		Name:  "random",
		Value: "random",
	})

	commands["start"] = &discordgo.ApplicationCommand{
		Name:        "start",
		Description: "join channel and start playing music",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "set",
				Description: "select which music set",
				Required:    true,
				Choices:     choices,
			},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "wacky",
				Description: "turn on wacky",
				Required:    false,
			},
		},
	}

	commandHandlers["start"] = func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		//Hack becuase this is boned
		cmd := i.ApplicationCommandData().Options[0].StringValue()
		log.Printf("Running start command %s\n", cmd)
		if cmd == "random" {
			vibeSets[vibesKeys[rand.Intn(len(vibesKeys)-1)]].startVibeCmd(s, i)
			return
		}

		for k, v := range vibeSets {
			if cmd == k {
				log.Printf("%s Start command matched %s trying to start vibing\n", i.ID, k)
				err := v.startVibeCmd(s, i)
				if err != nil {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: err.Error(),
						},
					})
					log.Printf("%s Error in startVibeCmd:%v\n", i.ID, err)
				}
				break
			}
		}
	}

	return commandSet{commands, commandHandlers}
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
	client := dbClient
	return client.Update(func(tx *bolt.Tx) error {
		b, _ := json.Marshal(info)
		return tx.Bucket(bucketName).Put(
			[]byte(id), []byte(b),
		)
	})
}

type voiceLock struct {
	lock    *semaphore.Weighted
	kill    chan bool
	channel string
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
		kill:    make(chan bool),
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
		for _, uid := range users {
			if uid == userID {
				return channel.ID, nil
			}
		}
	}

	return "", errors.New("could not find user")
}

func joinCaller(
	s *discordgo.Session, i *discordgo.InteractionCreate,
) (voice *discordgo.VoiceConnection, err error) {
	guild, err := s.State.Guild(i.GuildID)
	if err != nil {
		return nil, fmt.Errorf("could not find your discord server")

	}

	targetChannel, err := getUserChannel(i.GuildID, i.Member.User.ID, guild.Channels)
	if err != nil {
		return nil, fmt.Errorf("must be in a channel on the target server to vibe")
	}

	return s.ChannelVoiceJoin(i.GuildID, targetChannel, false, true)
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

// Gross
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
	defer rand.Seed(time.Now().Unix())
	return sets[rand.Intn(len(sets))]
}

func (i *guildInfo) startVibing(
	invoker vibes.Invoker, v *discordgo.VoiceConnection,
	g *discordgo.Guild, invert bool,
) {
	sets, err := invoker.GetSets()
	if err != nil {
		fmt.Printf("ERROR getting sets %s\n", err)
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
			if !bellPlayed && offsetTime(i.Offset).Minute() == 0 {
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
				<-done
				v.Speaking(false)
				encodingSession.Cleanup()
			}

			hour := offsetTime(i.Offset).Hour()

			if invert {
				if hour >= 12 {
					hour = hour - 12
				} else {
					hour = hour + 12
				}
			}

			stream, err := invoker.GetSampleStream(
				hour, randomGame(sets, i.Offset), i.City, i.Country,
			)
			offsetStart = true
			if err != nil {
				return err
			}
			defer stream.Close()

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
			defer encodingSession.Cleanup()
			select {
			case <-done:
				break
			case <-vl.kill:
				return fmt.Errorf("killing exsiting")
			}

			return nil
		}()
		if err != nil {
			fmt.Printf("%v\n", err)
			return
		}
	}
}

func defualtResponse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Processing...",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func setupVibeCmd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	defualtResponse(s, i)

	country := i.ApplicationCommandData().Options[0].StringValue()
	city := i.ApplicationCommandData().Options[1].StringValue()
	timeOffsetStr := i.ApplicationCommandData().Options[2].StringValue()

	offsetHour, err := strconv.Atoi(timeOffsetStr[:2])
	if err != nil || offsetHour > 14 || offsetHour < -12 {
		message := "Unable to save to DB"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &message,
		})
		return
	}

	err = setGuildInfo(i.GuildID, guildInfo{
		Country: country, City: city, Offset: timeOffsetStr,
	})
	if err != nil {
		message := "Unable to save to DB"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &message,
		})
		return
	}

	message := "created server info in DB!"
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func guildInfoCmd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	defualtResponse(s, i)

	info := getGuildInfo(i.GuildID)
	if info == nil {
		message := "no sever info in my DB"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &message,
		})

		return
	}

	message := fmt.Sprintf(
		"info %v\n hour: %d",
		info, offsetTime(info.Offset).Hour(),
	)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	})
}

func (v *vibeInfo) startVibeCmd(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	log.Printf("%s Start vibing entered\n", i.ID)
	defualtResponse(s, i)

	var wacky bool
	if len(i.ApplicationCommandData().Options) > 1 {
		wacky = i.ApplicationCommandData().Options[1].BoolValue()
	} else {
		wacky = false
	}
	log.Printf("%s Wacky %v\n", i.ID, wacky)

	if inVoice(i.GuildID) {
		log.Printf("%s Is currently in voice leaving", i.ID)
		vl := getVoiceLock(i.GuildID)
		vl.kill <- true
		deleteVoiceLock(i.GuildID)
	}

	log.Printf("%s Getting guild info", i.ID)
	info := getGuildInfo(i.GuildID)
	if info == nil {
		return fmt.Errorf("please setup server info first check help")
	}
	log.Printf("%s Guild gotten", i.ID)

	log.Printf("%s Joining call", i.ID)
	voice, err := joinCaller(s, i)
	if err != nil {
		return fmt.Errorf("unable to find you in a channel! Err: %v", err)
	}
	log.Printf("%s Joined called", i.ID)

	g, _ := s.Guild(i.GuildID)

	log.Printf("%s STARTING THE VIBING", i.ID)
	go info.startVibing(v.invoker, voice, g, wacky)

	message := fmt.Sprintf("we %sing now", strings.TrimSuffix(v.command, "e"))
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	})

	return nil
}

func stopVibeCmd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	defualtResponse(s, i)

	if !inVoice(i.GuildID) {
		message := "no vibes are happening right now"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &message,
		})
		return
	}

	deleteVoiceLock(i.GuildID)
	s.ChannelVoiceJoin(i.GuildID, "", true, true)

	message := "ok vibes stopped"
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	})
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

func commandsEqual(a, b *discordgo.ApplicationCommand) bool {
	aJson, _ := json.Marshal(a.Options)
	bJson, _ := json.Marshal(b.Options)
	return a.Name == b.Name && a.Description == b.Description && bytes.Equal(aJson, bJson)
}

func main() {
	token := strings.Replace(os.Getenv("DISCORD_AUTH"), "\"", "", -1)
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal("unable to create new discord instance ", err)
	}

	cs := createCommandSet(s)

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		log.Printf("Command gotten %s\n", i.ApplicationCommandData().Name)
		if h, ok := cs.handlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		s.UpdateListeningStatus("I use slash commands now")
		log.Println("Bot is up!")
	})

	// Register the messageCreate func as a callback for MessageCreate events.
	s.AddHandler(voiceStateUpdate)

	// Open a websocket connection to Discord and begin listening.
	if err := s.Open(); err != nil {
		log.Fatal("error opening connection,", err)
	}
	defer s.Close()

	// Clear existing commands
	existingCmds, _ := s.ApplicationCommands(s.State.User.ID, "")
	// delete deleted commandss
	for _, v := range existingCmds {
		if _, ok := cs.commands[v.Name]; !ok {
			err := s.ApplicationCommandDelete(v.ApplicationID, "", v.ID)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	// Edit updated commands
	for _, v := range existingCmds {
		cmd := cs.commands[v.Name]
		if _, ok := cs.commands[v.Name]; ok {
			if !commandsEqual(v, cmd) {
				if _, err := s.ApplicationCommandEdit(v.ApplicationID, "", v.ID, cmd); err != nil {
					log.Fatalf("Cannot edit '%v' command: %v", v.Name, err)
				}
			}
			delete(cs.commands, v.Name)
		}
	}

	// Create new commands
	for _, cmd := range cs.commands {
		if _, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd); err != nil {
			log.Fatalf("Cannot create '%v' command: %v", cmd, err)
		}
	}

	// Wait here until CTRL-C or other term signal is received.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("Gracefully shutdowning")
}
