package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/bwmarrin/discordgo"
)

const (
	secretName = "discord-bot-token"
)

var (
	err        error
	discord    *discordgo.Session
	ec2Client  *ec2.Client
	instanceId = os.Getenv("INSTANCE_ID")
)

func main() {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Fail to load aws config. %v", err)
	}

	smClient := secretsmanager.NewFromConfig(cfg)

	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}
	result, err := smClient.GetSecretValue(context.TODO(), input)
	if err != nil {
		log.Fatalf("Fail to get secret value. %v", err)
	}

	// JSONデータをパースしてマップに変換
	var secretData map[string]interface{}
	err = json.Unmarshal([]byte(*result.SecretString), &secretData)
	if err != nil {
		log.Fatalf("Failed to parse secret data. %v", err)
	}
	// シークレット値
	token := secretData[secretName].(string)

	discord, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Fail to create discord session. %v", err)
	}

	discord.AddHandler(onMessageCreate)

	discord.Identify.Intents = discordgo.IntentsGuildMessages

	err = discord.Open()
	if err != nil {
		log.Fatalf("Fail to open connection. %v", err)
	}

	ec2Client = ec2.NewFromConfig(cfg)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sig

	err = discord.Close()
	if err != nil {
		log.Fatalf("Fail to close connection. %v", err)
	}
}

func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// ボットからのメッセージを無視
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Content == "/minecraft status" {
		state, err := getInstanceStateCode()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: Fail to get instance state. "+err.Error())
			return
		}
		message, err := convertMessage(state)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: Fail to convert state. "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, message)
	}

	if m.Content == "/minecraft ip" {
		ip, err := getPublicIpAddress()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: Fail to get instance public ip address. "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, ip)
	}

	if m.Content == "/minecraft start" {
		// 停止中か確認
		instanceState, err := getInstanceStateCode()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: Fail to get instance state. "+err.Error())
			return
		}
		if *instanceState == 0 {
			s.ChannelMessageSend(m.ChannelID, "Instance is just starting up.")
			return
		}
		if *instanceState == 16 {
			s.ChannelMessageSend(m.ChannelID, "Instance is already running.")
			return
		}
		if *instanceState == 32 || *instanceState == 48 {
			s.ChannelMessageSend(m.ChannelID, "Error: Instance has been deleted.")
			return
		}
		if *instanceState == 64 {
			s.ChannelMessageSend(m.ChannelID, "Instance is stopping. Wait a few minutes and start it up again.")
			return
		}

		// インスタンス起動
		err = startInstance()
		s.ChannelMessageSend(m.ChannelID, "Instance starting... Wait few second.")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Start instance is error. "+err.Error())
			return
		}
		// 起動するまで待つ
		err = pollingStartInstance()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Start instance is error. "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "Instance is running.")
		// 起動したらPublic IP Addressを取得する
		ip, err := getPublicIpAddress()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: Fail to get instance public ip address. "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "IP Address: "+ip)
	}

	if m.Content == "/minecraft stop" {
		// 起動中か確認
		instanceState, err := getInstanceStateCode()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: Fail to get instance state. "+err.Error())
			return
		}
		if *instanceState == 0 {
			s.ChannelMessageSend(m.ChannelID, "Instance is just starting up.")
			return
		}
		if *instanceState == 32 || *instanceState == 48 {
			s.ChannelMessageSend(m.ChannelID, "Error: Instance has been deleted.")
			return
		}
		if *instanceState == 64 {
			s.ChannelMessageSend(m.ChannelID, "Instance is just stopping.")
			return
		}
		if *instanceState == 80 {
			s.ChannelMessageSend(m.ChannelID, "Instance is already stopped.")
		}

		// インスタンス停止
		err = stopInstance()
		s.ChannelMessageSend(m.ChannelID, "Instance stopping... Wait few second.")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Stop instance is error. "+err.Error())
			return
		}
		// 停止するまで待つ
		err = pollingStopInstance()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Stop instance is error. "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "Instance is stopped.")
	}
}

func getInstanceStateCode() (*int32, error) {
	instanceStatus, err := ec2Client.DescribeInstanceStatus(context.TODO(), &ec2.DescribeInstanceStatusInput{
		IncludeAllInstances: aws.Bool(true),
		InstanceIds:         []string{instanceId},
	})
	if err != nil {
		return nil, err
	}
	if len(instanceStatus.InstanceStatuses) != 1 {
		return nil, errors.New("fail to describe instance")
	}
	instanceStateCode := instanceStatus.InstanceStatuses[0].InstanceState.Code
	return instanceStateCode, nil
}

func startInstance() error {
	_, err = ec2Client.StartInstances(context.TODO(), &ec2.StartInstancesInput{
		InstanceIds: []string{instanceId},
	})
	return err
}

func stopInstance() error {
	_, err = ec2Client.StopInstances(context.TODO(), &ec2.StopInstancesInput{
		InstanceIds: []string{instanceId},
	})
	return err
}

func pollingInstanceState(expectCode int32, errorCh chan<- error) {
	for {
		stateCode, err := getInstanceStateCode()
		if err != nil {
			errorCh <- err
		}
		if *stateCode == expectCode {
			errorCh <- nil
		}
		time.Sleep(5 * time.Second)
	}
}

func pollingStartInstance() error {
	errorCh := make(chan error)
	timeout := time.After(60 * time.Second)

	go pollingInstanceState(16, errorCh)
	for {
		select {
		case err = <-errorCh:
			return err
		case <-timeout:
			return errors.New("start instance is timeout")
		}
	}
}

func pollingStopInstance() error {
	errorCh := make(chan error)
	timeout := time.After(120 * time.Second)

	go pollingInstanceState(80, errorCh)
	for {
		select {
		case err = <-errorCh:
			return err
		case <-timeout:
			return errors.New("stop instance is timeout")
		}
	}
}

func convertMessage(state *int32) (string, error) {
	if *state == 0 {
		return "Instance is pending", nil
	}
	if *state == 16 {
		return "Instance is running", nil
	}
	if *state == 32 {
		return "Instance is shutting down", nil
	}
	if *state == 48 {
		return "Instance is terminated", nil
	}
	if *state == 64 {
		return "Instance is stopping", nil
	}
	if *state == 80 {
		return "Instance is stopped", nil
	}
	return "", fmt.Errorf("instane is abnormal. StateCode: %d", *state)
}

func getPublicIpAddress() (string, error) {
	res, err := ec2Client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceId},
	})
	if err != nil {
		return "", err
	}
	if len(res.Reservations) != 1 {
		return "", errors.New("fail to describe instance, reservation is not one")
	}
	reservation := res.Reservations[0]
	if len(reservation.Instances) != 1 {
		return "", errors.New("fail to describe instance, instance is not one")
	}
	instance := reservation.Instances[0]
	return *instance.PublicIpAddress, nil
}
