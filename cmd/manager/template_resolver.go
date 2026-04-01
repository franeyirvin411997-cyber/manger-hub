package main

import (
	"encoding/json"
	"strings"

	"multi_node_platform/pkg/models"
)

// ResolvedApp 表示解析后的单个应用信息，发往 Node 端
type ResolvedApp struct {
	Identifier string   `json:"identifier"`
	RunArgs    []string `json:"run_args"`
}

// resolveAppCommands 将前端传来的应用 ID 列表和配置，根据数据库中的 AppTemplate 渲染成可执行的 Docker 参数列表
func resolveAppCommands(appIdentifiers []string, appConfigs map[string]string, groupID string) ([]ResolvedApp, error) {
	var resolved []ResolvedApp

	// 把 groupID 也作为默认参数注入，供 device 等字段使用
	mergedConfigs := make(map[string]string)
	for k, v := range appConfigs {
		mergedConfigs[k] = v
	}
	mergedConfigs["device"] = groupID
	mergedConfigs["group_id"] = groupID

	for _, id := range appIdentifiers {
		var tmpl models.AppTemplate
		if err := models.DB.Where("identifier = ?", id).First(&tmpl).Error; err != nil {
			// 如果没找到模板，这里采取一个非常保守的回退：启动 alpine sleep，但不阻塞主流程
			resolved = append(resolved, ResolvedApp{
				Identifier: id,
				RunArgs:    []string{"alpine", "sleep", "3600"},
			})
			continue
		}

		// 解析模板数组
		var argsTemplate []string
		if err := json.Unmarshal([]byte(tmpl.CommandTemplate), &argsTemplate); err != nil {
			// 如果模板有问题，回退
			resolved = append(resolved, ResolvedApp{
				Identifier: id,
				RunArgs:    []string{"alpine", "sleep", "3600"},
			})
			continue
		}

		// 替换变量 {{key}}
		var finalArgs []string
		for _, argTmpl := range argsTemplate {
			arg := argTmpl
			for key, val := range mergedConfigs {
				placeholder := "{{" + key + "}}"
				// 也可以支持带应用前缀的变量，比如 traffmonetizer_token
				prefixedPlaceholder := "{{" + id + "_" + key + "}}"
				arg = strings.ReplaceAll(arg, placeholder, val)
				arg = strings.ReplaceAll(arg, prefixedPlaceholder, val)
			}
			finalArgs = append(finalArgs, arg)
		}

		resolved = append(resolved, ResolvedApp{
			Identifier: id,
			RunArgs:    finalArgs,
		})
	}

	return resolved, nil
}
