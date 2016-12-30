package bitmovin

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/NYTimes/video-transcoding-api/db"
	"github.com/NYTimes/video-transcoding-api/provider"
	"github.com/bitmovin/bitmovin-go/bitmovin"
	"github.com/bitmovin/bitmovin-go/bitmovintypes"
	"github.com/bitmovin/bitmovin-go/models"
	"github.com/bitmovin/bitmovin-go/services"
	"github.com/bitmovin/video-transcoding-api/config"
)

// Name is the name used for registering the bitmovin provider in the
// registry of providers.
const Name = "bitmovin"

var h264Levels = []bitmovintypes.H264Level{
	bitmovintypes.H264Level1,
	bitmovintypes.H264Level1b,
	bitmovintypes.H264Level1_1,
	bitmovintypes.H264Level1_2,
	bitmovintypes.H264Level1_3,
	bitmovintypes.H264Level2,
	bitmovintypes.H264Level2_1,
	bitmovintypes.H264Level2_2,
	bitmovintypes.H264Level3,
	bitmovintypes.H264Level3_1,
	bitmovintypes.H264Level3_2,
	bitmovintypes.H264Level4,
	bitmovintypes.H264Level4_1,
	bitmovintypes.H264Level4_2,
	bitmovintypes.H264Level5,
	bitmovintypes.H264Level5_1,
	bitmovintypes.H264Level5_2}

var errBitmovinInvalidConfig = provider.InvalidConfigError("missing Bitmovin api key. Please define the environment variable BITMOVIN_API_KEY set this value in the configuration file")

var s3UrlCloudRegionMap = map[string]bitmovintypes.AWSCloudRegion{
	"s3.amazonaws.com":                          bitmovintypes.AWSCloudRegionUSEast1,
	"s3-external-1.amazonaws.com":               bitmovintypes.AWSCloudRegionUSEast1,
	"s3.dualstack.us-east-1.amazonaws.com":      bitmovintypes.AWSCloudRegionUSEast1,
	"s3-us-west-1.amazonaws.com":                bitmovintypes.AWSCloudRegionUSWest1,
	"s3.dualstack.us-west-1.amazonaws.com":      bitmovintypes.AWSCloudRegionUSWest1,
	"s3-us-west-2.amazonaws.com":                bitmovintypes.AWSCloudRegionUSWest2,
	"s3.dualstack.us-west-2.amazonaws.com":      bitmovintypes.AWSCloudRegionUSWest2,
	"s3.ap-south-1.amazonaws.com":               bitmovintypes.AWSCloudRegionAPSouth1,
	"s3-ap-south-1.amazonaws.com":               bitmovintypes.AWSCloudRegionAPSouth1,
	"s3.dualstack.ap-south-1.amazonaws.com":     bitmovintypes.AWSCloudRegionAPSouth1,
	"s3.ap-northeast-2.amazonaws.com":           bitmovintypes.AWSCloudRegionAPNortheast2,
	"s3-ap-northeast-2.amazonaws.com":           bitmovintypes.AWSCloudRegionAPNortheast2,
	"s3.dualstack.ap-northeast-2.amazonaws.com": bitmovintypes.AWSCloudRegionAPNortheast2,
	"s3-ap-southeast-1.amazonaws.com":           bitmovintypes.AWSCloudRegionAPSoutheast1,
	"s3.dualstack.ap-southeast-1.amazonaws.com": bitmovintypes.AWSCloudRegionAPSoutheast1,
	"s3-ap-southeast-2.amazonaws.com":           bitmovintypes.AWSCloudRegionAPSoutheast2,
	"s3.dualstack.ap-southeast-2.amazonaws.com": bitmovintypes.AWSCloudRegionAPSoutheast2,
	"s3-ap-northeast-1.amazonaws.com":           bitmovintypes.AWSCloudRegionAPNortheast1,
	"s3.dualstack.ap-northeast-1.amazonaws.com": bitmovintypes.AWSCloudRegionAPNortheast1,
	"s3.eu-central-1.amazonaws.com":             bitmovintypes.AWSCloudRegionEUCentral1,
	"s3-eu-central-1.amazonaws.com":             bitmovintypes.AWSCloudRegionEUCentral1,
	"s3.dualstack.eu-central-1.amazonaws.com":   bitmovintypes.AWSCloudRegionEUCentral1,
	"s3-eu-west-1.amazonaws.com":                bitmovintypes.AWSCloudRegionEUWest1,
	"s3.dualstack.eu-west-1.amazonaws.com":      bitmovintypes.AWSCloudRegionEUWest1,
	"s3-sa-east-1.amazonaws.com":                bitmovintypes.AWSCloudRegionSAEast1,
	"s3.dualstack.sa-east-1.amazonaws.com":      bitmovintypes.AWSCloudRegionSAEast1,
}

type bitmovinProvider struct {
	client *bitmovin.Bitmovin
	config *config.Bitmovin
}

type bitmovinPreset struct {
	Video models.H264CodecConfiguration
	Audio models.AACCodecConfiguration
}

func (p *bitmovinProvider) CreatePreset(preset db.Preset) (string, error) {
	//Find a corresponding audio configuration that lines up, otherwise create it
	if strings.ToLower(preset.Audio.Codec) != "aac" {
		return "", fmt.Errorf("Unsupported Audio codec: %v", preset.Audio.Codec)
	}
	// Bitmovin supports H.264 and H.265, H.265 support can be added in the future
	if strings.ToLower(preset.Video.Codec) != "h264" {
		return "", fmt.Errorf("Unsupported Video codec: %v", preset.Video.Codec)
	}

	aac := services.NewAACCodecConfigurationService(p.client)
	response, err := aac.List(0, 1)
	if err != nil {
		return "", err
	}
	if response.Status == "ERROR" {
		return "", errors.New("")
	}
	totalCount := *response.Data.Result.TotalCount
	response, err = aac.List(0, totalCount-1)
	if err != nil {
		return "", err
	}
	if response.Status == "ERROR" {
		return "", errors.New("")
	}
	var audioConfigID string
	audioConfigs := response.Data.Result.Items
	bitrate, err := strconv.Atoi(preset.Audio.Bitrate)
	if err != nil {
		return "", err
	}
	for _, c := range audioConfigs {
		if *c.Bitrate == int64(bitrate) {
			audioConfigID = *c.ID
			break
		}
	}
	if audioConfigID == "" {
		temp := int64(bitrate)
		audioConfig := &models.AACCodecConfiguration{
			Bitrate:      &temp,
			SamplingRate: floatToPtr(48000.0),
		}
		resp, err := aac.Create(audioConfig)
		if err != nil {
			return "", err
		}
		if resp.Status == "ERROR" {
			return "", errors.New("")
		}
		audioConfigID = *resp.Data.Result.ID
	}
	//Create Video and add Custom Data element to point to the
	customData := make(map[string]interface{})
	customData["audio"] = audioConfigID
	h264Config, err := p.createVideoPreset(preset)
	h264Config.CustomData = customData
	h264 := services.NewH264CodecConfigurationService(p.client)
	respo, err := h264.Create(h264Config)
	if err != nil {
		return "", err
	}
	if respo.Status == "ERROR" {
		return "", errors.New("")
	}
	return *respo.Data.Result.ID, nil
}

func (p *bitmovinProvider) createVideoPreset(preset db.Preset) (*models.H264CodecConfiguration, error) {
	h264 := &models.H264CodecConfiguration{}
	profile := strings.ToLower(preset.Video.Profile)
	switch profile {
	case "high":
		h264.Profile = bitmovintypes.H264ProfileHigh
	case "main":
		h264.Profile = bitmovintypes.H264ProfileMain
	case "baseline":
		h264.Profile = bitmovintypes.H264ProfileBaseline
	case "":
		h264.Profile = bitmovintypes.H264ProfileMain
	default:
		return nil, fmt.Errorf("Unrecognized H264 Profile: %v", preset.Video.Profile)
	}
	foundLevel := false
	for _, l := range h264Levels {
		if l == bitmovintypes.H264Level(preset.Video.ProfileLevel) {
			h264.Level = l
			foundLevel = true
			break
		}
	}
	if !foundLevel {
		return nil, fmt.Errorf("Unrecognized H264 Level: %v", preset.Video.ProfileLevel)
	}
	if preset.Video.Width != "" {
		width, err := strconv.Atoi(preset.Video.Width)
		if err != nil {
			return nil, err
		}
		h264.Width = intToPtr(int64(width))
	}
	if preset.Video.Height != "" {
		height, err := strconv.Atoi(preset.Video.Height)
		if err != nil {
			return nil, err
		}
		h264.Height = intToPtr(int64(height))
	}

	if preset.Video.Bitrate == "" {
		return nil, errors.New("Video Bitrate must be set")
	}
	bitrate, err := strconv.Atoi(preset.Video.Bitrate)
	if err != nil {
		return nil, err
	}
	h264.Bitrate = intToPtr(int64(bitrate))
	if preset.Video.GopSize != "" {
		gopSize, err := strconv.Atoi(preset.Video.GopSize)
		if err != nil {
			return nil, err
		}
		h264.MaxGOP = intToPtr(int64(gopSize))
	}

	return h264, nil
}

func (p *bitmovinProvider) DeletePreset(presetID string) error {
	// Only delete the video preset, leave the audio preset.
	h264 := services.NewH264CodecConfigurationService(p.client)
	response, err := h264.Delete(presetID)
	if err != nil {
		return err
	}
	if response.Status == "ERROR" {
		return errors.New("")
	}
	return nil
}

func (p *bitmovinProvider) GetPreset(presetID string) (interface{}, error) {
	// Return a custom struct with the H264 and AAC config?
	h264 := services.NewH264CodecConfigurationService(p.client)
	response, err := h264.Retrieve(presetID)
	if err != nil {
		return nil, err
	}
	if response.Status == "ERROR" {
		return nil, errors.New("")
	}
	h264Config := response.Data.Result
	i, ok := h264Config.CustomData["audio"]
	if !ok {
		return nil, errors.New("No Audio configuration found for Video Preset")
	}
	s, ok := i.(string)
	if !ok {
		return nil, errors.New("Audio Configuration somehow not a string")
	}
	aac := services.NewAACCodecConfigurationService(p.client)
	audioResponse, err := aac.Retrieve(s)
	if err != nil {
		return nil, err
	}
	if audioResponse.Status == "ERROR" {
		return nil, errors.New("")
	}
	aacConfig := audioResponse.Data.Result
	preset := bitmovinPreset{
		Video: h264Config,
		Audio: aacConfig,
	}
	return preset, errors.New("Not implemented")
}

func (p *bitmovinProvider) Transcode(*db.Job) (*provider.JobStatus, error) {
	//Parse the input and set it up
	//It will be an s3 url so need to parse out the region and the bucket name

	// Setup the streams and start transcoding
	return nil, errors.New("Not implemented")
}

func (p *bitmovinProvider) JobStatus(*db.Job) (*provider.JobStatus, error) {
	// If the transcoding is finished, start manifest generation, wait (because it is fast),
	// and then return done, otherwise send the status of the transcoding
	return nil, errors.New("Not implemented")
}

func (p *bitmovinProvider) CancelJob(jobID string) error {
	// stop the job
	return errors.New("Not implemented")
}

func (p *bitmovinProvider) Healthcheck() error {
	// unknown
	return errors.New("Not implemented")
}

func (p *bitmovinProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		InputFormats:  []string{"prores", "h264"},
		OutputFormats: []string{"mp4", "hls"},
		Destinations:  []string{"s3"},
	}
}

func bitmovinFactory(cfg *config.Config) (provider.TranscodingProvider, error) {
	if cfg.Bitmovin.APIKey == "" {
		return nil, errBitmovinInvalidConfig
	}
	client := bitmovin.NewBitmovin(cfg.Bitmovin.APIKey, cfg.Bitmovin.Endpoint, int64(cfg.Bitmovin.Timeout))
	return &bitmovinProvider{client: client, config: cfg.Bitmovin}, nil
}

func parseS3URL(input string) (fileName string, bucketName string, cloudRegion bitmovintypes.AWSCloudRegion, err error) {
	u, err := url.Parse(input)
	if err != nil {
		return "", "", bitmovintypes.AWSCloudRegion(""), err
	}
	s := strings.Split(u.Path, "/")
	fileName = s[len(s)-1]
	bucketName = strings.TrimSuffix(u.Path, "/"+fileName)
	bucketName = strings.TrimPrefix(bucketName, "/")
	cloudRegion, ok := s3UrlCloudRegionMap[u.Host]
	if !ok {
		return "", "", bitmovintypes.AWSCloudRegion(""), fmt.Errorf("Unable to determine AWS Region from Host: %v", u.Host)
	}
	return
}

func stringToPtr(s string) *string {
	return &s
}

func intToPtr(i int64) *int64 {
	return &i
}

func boolToPtr(b bool) *bool {
	return &b
}

func floatToPtr(f float64) *float64 {
	return &f
}
