// https://marcofranssen.nl/build-a-go-webserver-on-http-2-using-letsencrypt

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/basicrum/front_basicrum_go/backup"
	_ "github.com/basicrum/front_basicrum_go/dao/clickhouse"
	_ "github.com/basicrum/front_basicrum_go/dao/mysql"
	_ "github.com/basicrum/front_basicrum_go/dao/redis"
	_ "github.com/basicrum/front_basicrum_go/dao/rethinkdb"

	"github.com/basicrum/front_basicrum_go/geoip"
	"github.com/basicrum/front_basicrum_go/geoip/cloudflare"
	"github.com/basicrum/front_basicrum_go/geoip/maxmind"
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
	// configuration file
	configfile := flag.String("config", "front_basicrum_go.conf", "Path to config file.")
	// reset the database. Empty all the tables
	reset := flag.Bool("reset", false, "force database reset")
	// needed if there exists several database versions and an upgrade is needed
	upgrade := flag.Bool("upgrade", false, "perform database version upgrade")
	// necessary to check whether the database is initiallized(created)
	noInit := flag.Bool("no_init", true, "check that database exists but don't create if missing")
	flag.Parse()
	slog.Info("Using database flags as:", "reset", *reset, "upgrade", *upgrade, "no init", *noInit)
	curwd, err := os.Getwd()
	if err != nil {
		slog.Error("Couldn't get current working directory: ", "error", err)
		os.Exit(1)
	}
	*configfile = toAbsolutePath(curwd, *configfile)
	slog.Info(fmt.Sprintf("Using config from '%s'", *configfile))

	var config configType

	// proccess config file
	if file, err := os.Open(*configfile); err != nil {
		slog.Error("Failed to read config file: ", err)
		os.Exit(1)
	} else {
		jr := jcr.New(file)
		if err = json.NewDecoder(jr).Decode(&config); err != nil {
			switch jerr := err.(type) {
			case *json.UnmarshalTypeError:
				lnum, cnum, _ := jr.LineAndChar(jerr.Offset)
				slog.Error(fmt.Sprintf("Unmarshall error in config file in %s at %d:%d (offset %d bytes): %s",
					jerr.Field, lnum, cnum, jerr.Offset, jerr.Error()))
				os.Exit(1)
			case *json.SyntaxError:
				lnum, cnum, _ := jr.LineAndChar(jerr.Offset)
				slog.Error(fmt.Sprintf("Syntax error in config file at %d:%d (offset %d bytes): %s",
					lnum, cnum, jerr.Offset, jerr.Error()))
				os.Exit(1)
			default:
				slog.Error("Failed to parse config file: ", "error", err)
				os.Exit(1)
			}
		}
		file.Close()
	}
	/*err1:=store.Store.InitDb(config.Store, true)
	if err1 != nil{
		slog.Error(err1.Error())
	}*/
	//vs:= store.Store.GetDbVersion()
	//slog.Info("", "DATABASE VERSION", vs)
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
				slog.Error("Database not found.")
				os.Exit(1)
			}
			slog.Info("Database not found. Creating.")
			err = store.Store.InitDb(config.Store, true)
			if err == nil {
				slog.Info("Database successfully created.")
				//created = true
			} else {
				slog.Warn("Database failed to initialize", "error", err)
			}
		} else if strings.Contains(err.Error(), "Invalid database version") {
			msg := "Wrong DB version: expected " + strconv.Itoa(adapterVersion) + ", got " +
				strconv.Itoa(databaseVersion) + "."

			if *reset {
				slog.Info(msg, "", "Reset Requested. Dropping and recreating the database.")
				err = store.Store.InitDb(config.Store, true)
				if err == nil {
					slog.Info("Database successfully reset.")
				}
			} else if *upgrade {
				if databaseVersion > adapterVersion {
					slog.Error(msg + "Unable to upgrade: database has greater version than the adapter.")
					os.Exit(1)
				}
				slog.Info(msg + "Upgrading the database.")
				err = store.Store.UpgradeDb(config.Store)
				if err == nil {
					slog.Info("Database successfully upgraded.")
				}
			} else {
				slog.Error(msg + "Use --reset to reset, --upgrade to upgrade.")
				os.Exit(1)
			}
		} else {
			slog.Error("Failed to init DB adapter:" + err.Error())
			os.Exit(1)
		}
	} else if *reset {
		slog.Info("Reset requested. Dropping and recreating the database.")
		slog.Warn("Database reset requested. Dropping and recreating the database. This will lead to loss of the data")
		err = store.Store.InitDb(config.Store, true)
		if err == nil {
			slog.Info("Database successfully reset.")
		} else {
			slog.Warn("Database reset failed", "error", err)
		}
	} else {
		slog.Info("Database exists, version is correct.")
	}

	// We need to get the Regexes from here: https://github.com/ua-parser/uap-core/blob/master/regexes.yaml
	userAgentParser, err := uaparser.NewFromBytes(userAgentRegularExpressions)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
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
		slog.Error(err.Error())
		os.Exit(1)
	}

	subscriptionService := makeSubscriptionService(&config)
	err = subscriptionService.Load()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
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
		slog.Error(err.Error())
		os.Exit(1)
	}

	go processingService.Run()
	startServers(servers)
	if err := stopServers(servers, backupService); err != nil {
		slog.Error(fmt.Sprintf("Shutdown Failed:%+v", err))
		os.Exit(1)
	}
	slog.Info("Servers exited properly")
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
				slog.Info(fmt.Sprintf("error start server index[%v] err[%v]\n", index, err))
			}
		}(srv, index)
	}
	slog.Info("Servers started")

	<-done
}

func stopServers(servers []*server.Server, backupService backup.IBackup) error {
	slog.Info("Stopping servers...")

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
				slog.Error(fmt.Sprintf("Server Shutdown Failed:%+v", err))
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
