package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
)

func sameAIModel(a, b aiConfig) bool {
	return sameAIEndpoint(a, b) && a.Model == b.Model && a.APIKey == b.APIKey
}

func imageRouteError(c aiConfig, independent bool, err error) error {
	if independent {
		return fmt.Errorf("图片批改失败，独立图片接口的模型 %s 必须支持识图；请检查或更换该模型：%w", c.Model, err)
	}
	return fmt.Errorf("图片批改失败，未配置独立图片接口，当前使用文字模型 %s；它必须支持识图，请更换文字模型或配置独立图片接口：%w", c.Model, err)
}

func probeVision(ctx context.Context, c aiConfig) error {
	// A generic CONNECTED response only proves chat works: a relay can drop the
	// image silently. Keep the random answer exclusively in pixels, not the prompt.
	names := []string{"red", "green", "blue", "yellow"}
	colors := []color.RGBA{{255, 0, 0, 255}, {0, 160, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}}
	var choices [6]byte
	if _, err := rand.Read(choices[:]); err != nil {
		return errors.New("无法生成识图测试")
	}
	picture := image.NewRGBA(image.Rect(0, 0, 496, 96))
	draw.Draw(picture, picture.Bounds(), image.White, image.Point{}, draw.Src)
	var expected []string
	for i, choice := range choices {
		index := int(choice) % len(colors)
		draw.Draw(picture, image.Rect(16+i*80, 16, 80+i*80, 80), image.NewUniform(colors[index]), image.Point{}, draw.Src)
		expected = append(expected, names[index])
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		return err
	}
	parts := []any{
		map[string]any{"type": "text", "text": "Read the attached image. Name the colors of the six squares from left to right. Use only red, green, blue or yellow. Reply with exactly six English color words separated by spaces, without explanation. If you cannot see the image, say UNABLE."},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), "detail": "auto"}},
	}
	answer, err := callChat(ctx, c, []chatMessage{{Role: "user", Content: parts}}, 0, 4096, true)
	if err != nil {
		return fmt.Errorf("识图测试未通过：%w", err)
	}
	words := strings.FieldsFunc(strings.ToLower(answer), func(r rune) bool { return r < 'a' || r > 'z' })
	if strings.Join(words, " ") != strings.Join(expected, " ") {
		return errors.New("模型未正确识别测试图片，无法确认它支持识图；没有发送练习资料")
	}
	return nil
}

func ensureVision(ctx context.Context, c aiConfig, independent bool) error {
	if c.VisionVerified {
		return nil
	}
	if err := probeVision(ctx, c); err != nil {
		return err
	}
	settings.Lock()
	defer settings.Unlock()
	updated := settings.value
	current := updated
	currentIndependent := updated.Vision != nil && updated.Vision.Connected
	if currentIndependent {
		current = updated.Vision.config()
	}
	if independent != currentIndependent || !current.Connected || !sameAIModel(c, current) {
		return errors.New("测试期间接口配置已更改，请重试")
	}
	if independent {
		endpoint := *updated.Vision
		endpoint.VisionVerified = true
		updated.Vision = &endpoint
	} else {
		updated.VisionVerified = true
	}
	if err := settings.persist(updated); err != nil {
		return errors.New("识图测试通过，但无法加密保存验证结果，请检查程序目录写入权限")
	}
	settings.value = updated
	return nil
}
