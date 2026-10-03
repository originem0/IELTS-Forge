package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errAIOutputFormat = errors.New("模型输出不完整或格式无效")

func outputFormatError(message string) error {
	return fmt.Errorf("%w：%s", errAIOutputFormat, message)
}

func nonemptyText(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}
func stringArray(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if !nonemptyText(item) {
			return false
		}
	}
	return true
}

func validateAIJSONObject(value map[string]any, contract string) error {
	invalid := errors.New("模型返回的结构缺少可用内容或字段类型错误")
	if contract == "study-plan-json-v1" {
		phases, _ := value["phases"].([]any)
		if !nonemptyText(value["summary"]) || !stringArray(value["priorities"]) || len(phases) == 0 {
			return invalid
		}
		for _, entry := range phases {
			phase, ok := entry.(map[string]any)
			if !ok || !nonemptyText(phase["name"]) || !nonemptyText(phase["focus"]) {
				return invalid
			}
			days, ok := phase["days"].([]any)
			if !ok || len(days) != 7 {
				return invalid
			}
			for _, entry := range days {
				day, ok := entry.(map[string]any)
				if !ok {
					return invalid
				}
				if _, ok := day["note"].(string); !ok {
					return invalid
				}
				for key, limit := range map[string]float64{"writing": 1, "speaking": 2, "reading": 1, "listening": 1, "writingReview": 1, "writingRewrite": 1, "speakingReview": 1, "readingReview": 1, "listeningReview": 1, "languageMinutes": 1440, "reviewMinutes": 1440} {
					count, ok := day[key].(float64)
					if !ok || count < 0 || count > limit || count != float64(int(count)) {
						return invalid
					}
				}
			}
		}
		return nil
	}
	count := 0
	for _, kind := range []string{"speaking", "writing"} {
		items, _ := value[kind].([]any)
		for _, entry := range items {
			item, ok := entry.(map[string]any)
			if !ok {
				return invalid
			}
			if !stringArray(item["sourceKeys"]) {
				return invalid
			}
			if kind == "writing" {
				if !nonemptyText(item["domain"]) || !stringArray(item["collocations"]) || !stringArray(item["sentencePatterns"]) {
					return invalid
				}
				if len(item["collocations"].([]any))+len(item["sentencePatterns"].([]any)) == 0 {
					return invalid
				}
			} else {
				if !nonemptyText(item["title"]) || !stringArray(item["reusableTopics"]) || !stringArray(item["expressions"]) || !stringArray(item["answerFrames"]) {
					return invalid
				}
				if _, ok := item["personalCore"].(string); !ok {
					return invalid
				}
				if !nonemptyText(item["personalCore"]) && len(item["expressions"].([]any))+len(item["answerFrames"].([]any)) == 0 {
					return invalid
				}
			}
			count++
		}
	}
	if entries, exists := value["noContentSources"]; exists {
		outcomes, ok := entries.([]any)
		if !ok {
			return invalid
		}
		for _, entry := range outcomes {
			item, ok := entry.(map[string]any)
			if !ok || !nonemptyText(item["sourceKey"]) || !nonemptyText(item["reason"]) {
				return invalid
			}
			count++
		}
	}
	if count == 0 {
		return invalid
	}
	return nil
}

// Retry only output failures. Transport/authentication failures and provider
// refusals must not be mistaken for a format problem or trigger another call.
func callChatValidated(ctx context.Context, cfg aiConfig, messages []chatMessage, temperature float64, maxTokens int, contract string) (string, error) {
	content, _, err := callChatBudgeted(ctx, cfg, messages, temperature, maxTokens, contract, 2)
	return content, err
}

func callChatBudgeted(ctx context.Context, cfg aiConfig, messages []chatMessage, temperature float64, maxTokens int, contract string, budget int) (string, int, error) {
	if budget < 1 || budget > 2 {
		budget = 2
	}
	var err error
	for attempt := 0; attempt < budget; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", attempt, err
		}
		var content string
		content, err = callChat(ctx, cfg, messages, temperature, maxTokens, false)
		if err == nil {
			if validation := validateOutputContract(content, contract); validation != nil {
				err = outputFormatError(validation.Error())
			} else {
				return content, attempt + 1, nil
			}
		}
		if !errors.Is(err, errAIOutputFormat) {
			return "", attempt + 1, err
		}
		// Copy before appending so a retry never changes the caller's messages.
		messages = append(append([]chatMessage(nil), messages...), chatMessage{Role: "system", Content: "上次输出未通过完整性或格式校验。请重新给出完整答案，严格遵守要求的字段、章节与数据类型，不要输出解释或省略必要内容。"})
	}
	return "", budget, err
}
