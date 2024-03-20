// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package main is the main package of the application
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	pc "github.com/opiproject/opi-api/inventory/v1/gen/go"
	pe "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/bridge"
	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/taskmanager"
	"github.com/opiproject/opi-evpn-bridge/pkg/port"
	"github.com/opiproject/opi-evpn-bridge/pkg/svi"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/opiproject/opi-evpn-bridge/pkg/vrf"
	"github.com/opiproject/opi-smbios-bridge/pkg/inventory"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	// "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	// "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	ci_linux "github.com/opiproject/opi-evpn-bridge/pkg/LinuxCIModule"
	gen_linux "github.com/opiproject/opi-evpn-bridge/pkg/LinuxGeneralModule"
	ipu_linux "github.com/opiproject/opi-evpn-bridge/pkg/LinuxVendorModule/ipu"
	frr "github.com/opiproject/opi-evpn-bridge/pkg/frr"
	netlink "github.com/opiproject/opi-evpn-bridge/pkg/netlink"
	ipu_vendor "github.com/opiproject/opi-evpn-bridge/pkg/vendor_plugins/intel/p4runtime/p4translation"
)

const (
	configFilePath = "./"
)

var rootCmd = &cobra.Command{
	Use:   "opi-evpn-bridge",
	Short: "evpn bridge",
	Long:  "evpn bridge application",
	PreRunE: func(_ *cobra.Command, _ []string) error {
		return validateConfigs()
	},
	Run: func(_ *cobra.Command, _ []string) {

		taskmanager.TaskMan.StartTaskManager()

		err := infradb.NewInfraDB(config.GlobalConfig.DBAddress, config.GlobalConfig.Database)
		if err != nil {
			log.Println("error in creating db", err)
		}
		go runGatewayServer(config.GlobalConfig.GRPCPort, config.GlobalConfig.HTTPPort)

		defer func() {
			if err := infradb.Close(); err != nil {
				log.Fatal(err)
			}
		}()

		switch config.GlobalConfig.Buildenv {
		case "ipu":
			gen_linux.Init()
			ipu_linux.Init()
			frr.Init()
			netlink.Init()
			ipu_vendor.Init()
		case "ci":
			gen_linux.Init()
			ci_linux.Init()
			frr.Init()
		default:
			log.Fatal(" ERROR: Could not find Build env ")
		}

		// Create GRD VRF configuration during startup
		if err := createGrdVrf(); err != nil {
			log.Printf("Error in creating GRD VRF %+v\n", err)
		}

		runGrpcServer(config.GlobalConfig.GRPCPort, config.GlobalConfig.TLSFiles)
	},
}

// initialize the cobra configuration and bind the flags
func initialize() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&config.GlobalConfig.CfgFile, "config", "c", "config.yaml", "config file path")
	rootCmd.PersistentFlags().IntVar(&config.GlobalConfig.GRPCPort, "grpcport", 50151, "The gRPC server port")
	rootCmd.PersistentFlags().IntVar(&config.GlobalConfig.HTTPPort, "httpport", 8082, "The HTTP server port")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.TLSFiles, "tlsfiles", "", "TLS files in server_cert:server_key:ca_cert format.")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.DBAddress, "dbaddress", "127.0.0.1:6379", "db address in ip_address:port format")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.FRRAddress, "frraddress", "127.0.0.1", "Frr address in ip_address format, no port")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.Database, "database", "redis", "Database connection string")

	if err := viper.GetViper().BindPFlags(rootCmd.PersistentFlags()); err != nil {
		log.Printf("Error binding flags to Viper: %v\n", err)
		os.Exit(1)
	}
}

// initConfig read the config from file
func initConfig() {
	if config.GlobalConfig.CfgFile != "" {
		viper.SetConfigFile(config.GlobalConfig.CfgFile)
	} else {
		// Search config in the default location
		viper.AddConfigPath(configFilePath)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config.yaml")
	}
	config.LoadConfig()
}

const logfile string = "opi-evpn-bridge.log"

var logger *log.Logger

func setupLogger(filename string) {
	var err error
	filename = filepath.Clean(filename)
	out, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	logger = log.New(io.MultiWriter(out), "", log.Lshortfile|log.LstdFlags)
	log.SetOutput(logger.Writer())
}

// validateConfigs validates the config parameters
func validateConfigs() error {
	var err error

	grpcPort := viper.GetInt("grpcport")
	if grpcPort <= 0 || grpcPort > 65535 {
		err = fmt.Errorf("grpcPort must be a positive integer between 1 and 65535")
		return err
	}

	httpPort := viper.GetInt("httpport")
	if httpPort <= 0 || httpPort > 65535 {
		err = fmt.Errorf("httpPort must be a positive integer between 1 and 65535")
		return err
	}

	dbAddr := viper.GetString("dbaddress")
	_, port, err := net.SplitHostPort(dbAddr)
	if err != nil {
		err = fmt.Errorf("invalid DBAddress format. It should be in ip_address:port format")
		return err
	}

	dbPort, err := strconv.Atoi(port)
	if err != nil || dbPort <= 0 || dbPort > 65535 {
		err = fmt.Errorf("invalid db port. It must be a positive integer between 1 and 65535")
		return err
	}

	frrAddr := viper.GetString("frraddress")
	if net.ParseIP(frrAddr) == nil {
		err = fmt.Errorf("invalid FRRAddress format. It should be a valid IP address")
		return err
	}

	return nil
}

// main function
func main() {
	// setup file and console logger
	setupLogger(logfile)
	// initialize  cobra config
	initialize()
	// start the main cmd
	if err := rootCmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// runGrpcServer start the grpc server for all the components
func runGrpcServer(grpcPort int, tlsFiles string) {
	tp := utils.InitTracerProvider("opi-evpn-bridge")
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Panicf("Tracer Provider Shutdown: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Panicf("failed to listen: %v", err)
	}

	var serverOptions []grpc.ServerOption
	if tlsFiles == "" {
		log.Println("TLS files are not specified. Use insecure connection.")
	} else {
		log.Println("Use TLS certificate files:", tlsFiles)
		config, err := utils.ParseTLSFiles(tlsFiles)
		if err != nil {
			log.Panic("Failed to parse string with tls paths:", err)
		}
		log.Println("TLS config:", config)
		var option grpc.ServerOption
		if option, err = utils.SetupTLSCredentials(config); err != nil {
			log.Panic("Failed to setup TLS:", err)
		}
		serverOptions = append(serverOptions, option)
	}
	/*serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(
		otelgrpc.UnaryServerInterceptor(),
		logging.UnaryServerInterceptor(utils.InterceptorLogger(log.Default()),
			logging.WithLogOnEvents(
				logging.StartCall,
				logging.FinishCall,
				logging.PayloadReceived,
				logging.PayloadSent,
			),
		)),
	)*/
	s := grpc.NewServer(serverOptions...)

	bridgeServer := bridge.NewServer()
	portServer := port.NewServer()
	vrfServer := vrf.NewServer()
	sviServer := svi.NewServer()
	pe.RegisterLogicalBridgeServiceServer(s, bridgeServer)
	pe.RegisterBridgePortServiceServer(s, portServer)
	pe.RegisterVrfServiceServer(s, vrfServer)
	pe.RegisterSviServiceServer(s, sviServer)
	pc.RegisterInventoryServiceServer(s, &inventory.Server{})

	reflection.Register(s)

	log.Printf("gRPC server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
}

// runGatewayServer
func runGatewayServer(grpcPort int, httpPort int) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register gRPC server endpoint
	// Note: Make sure the gRPC server is running properly and accessible
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// TODO: add/replace with more/less registrations, once opi-api compiler fixed
	err := pc.RegisterInventoryServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf(":%d", grpcPort), opts)
	if err != nil {
		log.Panic("cannot register handler server")
	}

	// Start HTTP server (and proxy calls to gRPC server endpoint)
	log.Printf("HTTP Server listening at %v", httpPort)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", httpPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Panic("cannot start HTTP gateway server")
	}
}

// createGrdVrf creates the grd vrf with vni 0
func createGrdVrf() error {
	grdVrf, err := infradb.NewVrfWithArgs("//network.opiproject.org/vrfs/GRD", nil, nil, nil)
	if err != nil {
		log.Printf("CreateGrdVrf(): Error in initializing GRD VRF object %+v\n", err)
		return err
	}

	err = infradb.CreateVrf(grdVrf)
	if err != nil {
		log.Printf("CreateGrdVrf(): Error in creating GRD VRF object %+v\n", err)
		return err
	}

	return nil
}
