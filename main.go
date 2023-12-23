// https://marcofranssen.nl/build-a-go-webserver-on-http-2-using-letsencrypt

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/basicrum/front_basicrum_go/backup"
	"github.com/basicrum/front_basicrum_go/config"
	_ "github.com/basicrum/front_basicrum_go/dao/clickhouse"
	_ "github.com/basicrum/front_basicrum_go/dao/redis"
	_ "github.com/basicrum/front_basicrum_go/dao/rethinkdb"

	"github.com/basicrum/front_basicrum_go/geoip"
	"github.com/basicrum/front_basicrum_go/geoip/cloudflare"
	"github.com/basicrum/front_basicrum_go/geoip/maxmind"
	"github.com/basicrum/front_basicrum_go/logs"
	"github.com/basicrum/front_basicrum_go/server"
	"github.com/basicrum/front_basicrum_go/service"
	"github.com/basicrum/front_basicrum_go/service/subscription/caching"
	"github.com/basicrum/front_basicrum_go/service/subscription/disabled"
	"github.com/basicrum/front_basicrum_go/store"

	"github.com/ua-parser/uap-go/uaparser"
	"golang.org/x/sync/errgroup"

	// For stripping comments from JSON config
	jcr "github.com/tinode/jsonco"
)

//go:embed assets/uaparser_regexes.yaml
var userAgentRegularExpressions []byte

// Contentx of the configuration file

type Backup struct {
	Enabled          bool   `json:"BRUM_BACKUP_ENABLED,omitempty"`
	Directory        string `json:"BRUM_BACKUP_DIRECTORY,omitempty"`
	IntervalSeconds  uint32 `json:"BRUM_BACKUP_INTERVAL_SECONDS,omitempty"`
	CompressionType  string `json:"BRUM_COMPRESSION_TYPE,omitempty"`
	CompressionLevel string `json:"BRUM_COMPRESSION_LEVEL,omitempty"`
}

type Subscription struct {
	Enabled bool `json:"BRUM_SUBSCRIPTION_ENABLED"`
}
type configType struct {
	Store        json.RawMessage `json:"store_config"`
	Backup       `json:"backup"`
	Subscription `json:"subsription"`
	Server       json.RawMessage `json:"server"`
}

// nolint: revive
func main() {
	logFlags := flag.String("log_flags", "stdFlags",
		"Comma-separated list of log flags (as defined in https://golang.org/pkg/log/#pkg-constants without the L prefix)")
	configfile := flag.String("config", "front_basicrum_go.conf", "Path to config file.")
	reset := flag.Bool("reset", false, "force database reset")
	upgrade := flag.Bool("upgrade", false, "perform database version upgrade")
	noInit := flag.Bool("no_init", false, "check that database exists but don't create if missing")
	flag.Parse()
	logs.Init(os.Stderr, *logFlags)
	curwd, err := os.Getwd()
	if err != nil {
		logs.Err.Fatal("Couldn't get current working directory: ", err)
	}
	*configfile = toAbsolutePath(curwd, *configfile)
	logs.Info.Printf("Using config from '%s'", *configfile)

	_, err = config.GetStartupConfig()
	if err != nil {
		log.Fatal(err)
	}

	var config configType
	if file, err := os.Open(*configfile); err != nil {
		logs.Err.Fatal("Failed to read config file: ", err)
	} else {
		jr := jcr.New(file)
		if err = json.NewDecoder(jr).Decode(&config); err != nil {
			switch jerr := err.(type) {
			case *json.UnmarshalTypeError:
				lnum, cnum, _ := jr.LineAndChar(jerr.Offset)
				logs.Err.Fatalf("Unmarshall error in config file in %s at %d:%d (offset %d bytes): %s",
					jerr.Field, lnum, cnum, jerr.Offset, jerr.Error())
			case *json.SyntaxError:
				lnum, cnum, _ := jr.LineAndChar(jerr.Offset)
				logs.Err.Fatalf("Syntax error in config file at %d:%d (offset %d bytes): %s",
					lnum, cnum, jerr.Offset, jerr.Error())
			default:
				logs.Err.Fatal("Failed to parse config file: ", err)
			}
		}
		file.Close()
	}
	err = store.Store.Open(1, config.Store)
	defer store.Store.Close()
	adapterVersion := store.Store.GetAdapterVersion()
	databaseVersion := 0
	if store.Store.IsOpen() {
		databaseVersion = store.Store.GetDbVersion()
	}
	if err != nil {
		if strings.Contains(err.Error(), "Database not initialized") {
			if *noInit {
				log.Fatalln("Database not found.")
			}
			log.Println("Database not found. Creating.")
			err = store.Store.InitDb(config.Store, false)
			if err == nil {
				log.Println("Database successfully created.")
				//created = true
			}
		} else if strings.Contains(err.Error(), "Invalid database version") {
			msg := "Wrong DB version: expected " + strconv.Itoa(adapterVersion) + ", got " +
				strconv.Itoa(databaseVersion) + "."

			if *reset {
				log.Println(msg, "Reset Requested. Dropping and recreating the database.")
				err = store.Store.InitDb(config.Store, true)
				if err == nil {
					log.Println("Database successfully reset.")
				}
			} else if *upgrade {
				if databaseVersion > adapterVersion {
					log.Fatalln(msg, "Unable to upgrade: database has greater version than the adapter.")
				}
				log.Println(msg, "Upgrading the database.")
				err = store.Store.UpgradeDb(config.Store)
				if err == nil {
					log.Println("Database successfully upgraded.")
				}
			} else {
				log.Fatalln(msg, "Use --reset to reset, --upgrade to upgrade.")
			}
		} else {
			log.Fatalln("Failed to init DB adapter:", err)
		}
	} else if *reset {
		log.Println("Reset requested. Dropping and recreating the database.")
		err = store.Store.InitDb(config.Store, true)
		if err == nil {
			log.Println("Database successfully reset.")
		}
	} else {
		log.Println("Database exists, version is correct.")
	}
	stats := store.Store.DbStats()
	fmt.Println(stats())
	// We need to get the Regexes from here: https://github.com/ua-parser/uap-core/blob/master/regexes.yaml
	userAgentParser, err := uaparser.NewFromBytes(userAgentRegularExpressions)
	if err != nil {
		log.Fatal(err)
	}

	/*conn, err := dao.NewConnection(
		daoServer,
		daoAuth,
	)
	if err != nil {
		log.Fatal(err)
	}

	daoService := dao.New(
		conn,
		dao.Opts(sConf.Database.TablePrefix),
	)

	migrateDaoService := dao.NewMigrationDAO(
		daoServer,
		daoAuth,
		dao.Opts(sConf.Database.TablePrefix),
	)

	err = migrateDaoService.Migrate()
	if err != nil {
		log.Fatalf("migrate database ERROR: %+v", err)
	}*/

	geopIPService := geoip.NewComposite(
		cloudflare.New(),
		maxmind.New(),
	)

	compressionFactory := backup.NewCompressionWriterFactory(config.Backup.Enabled, backup.Compression(config.Backup.CompressionType), backup.CompressionLevel(config.Backup.CompressionLevel))
	backupInterval := time.Duration(config.Backup.IntervalSeconds) * time.Second
	backupService, err := backup.New(config.Backup.Enabled, backupInterval, config.Backup.Directory, compressionFactory)
	if err != nil {
		log.Fatal(err)
	}

	subscriptionService := makeSubscriptionService(&config)
	err = subscriptionService.Load()
	if err != nil {
		log.Fatal(err)
	}
	rumEventFactory := service.NewRumEventFactory(userAgentParser, geopIPService)
	processingService := service.New(
		rumEventFactory,
		store.Store.GetAdapter(),
		subscriptionService,
		backupService,
	)
	serverFactory := server.NewFactory(processingService, backupService)
	servers, err := serverFactory.Build(config.Server)
	if err != nil {
		log.Fatal(err)
	}

	go processingService.Run()
	startServers(servers)
	if err := stopServers(servers, backupService); err != nil {
		log.Fatalf("Shutdown Failed:%+v", err)
	}
	log.Print("Servers exited properly")
}

func makeSubscriptionService(conf *configType) service.ISubscriptionService {
	if !conf.Subscription.Enabled {
		return disabled.New()
	}
	return caching.New(store.Store.GetAdapter())
}

func startServers(servers []*server.Server) {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	for index, srv := range servers {
		go func(srv *server.Server, index int) {
			if err := srv.Serve(); err != nil {
				log.Printf("error start server index[%v] err[%v]\n", index, err)
			}
		}(srv, index)
	}
	log.Print("Servers started")

	<-done
}

func stopServers(servers []*server.Server, backupService backup.IBackup) error {
	log.Print("Stopping servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	g, ctx := errgroup.WithContext(ctx)

	for _, srv := range servers {
		serverCopy := srv
		g.Go(func() error {
			if err := serverCopy.Shutdown(ctx); err != nil {
				log.Printf("Server Shutdown Failed:%+v", err)
				return err
			}
			return nil
		})
	}

	g.Go(func() error {
		backupService.Flush()
		return nil
	})

	// wait for all parallel jobs to finish
	return g.Wait()
}

// Convert relative filepath to absolute.
func toAbsolutePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(base, path))
}
