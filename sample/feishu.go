package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// 通过手机号获取飞书用户 ID
func getUserIdByMobile(client *lark.Client, mobile string) (string, error) {
	req := larkcontact.NewBatchGetIdUserReqBuilder().
		UserIdType(`open_id`).
		Body(larkcontact.NewBatchGetIdUserReqBodyBuilder().
			Mobiles([]string{mobile}).
			IncludeResigned(true).
			Build()).Build()
	// 发起请求
	resp, err := client.Contact.V3.User.BatchGetId(context.Background(), req)
	if err != nil {
		fmt.Println(err)
		return "", fmt.Errorf("获取用户ID失败: %w", err)
	}
	if !resp.Success() {
		fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
		return "", fmt.Errorf("获取用户ID失败: %d", resp.CodeError.Code)
	}
	fmt.Println(larkcore.Prettify(resp))
	return *resp.Data.UserList[0].UserId, nil
}

// 通过用户ID获取用户信息
func getUserInfoByUserId(client *lark.Client, userId string) (string, error) {
	req := larkcontact.NewGetUserReqBuilder().
		UserId(userId).
		UserIdType(`open_id`).
		DepartmentIdType(`open_department_id`).
		Build()
	resp, err := client.Contact.V3.User.Get(context.Background(), req)
	if err != nil {
		fmt.Println(err)
		return "", fmt.Errorf("获取用户ID失败: %w", err)
	}
	if !resp.Success() {
		fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
		return "", fmt.Errorf("获取用户ID失败: %d", resp.CodeError.Code)
	}

	fmt.Println(larkcore.Prettify(resp))

	departmentId := ""
	for _, depId := range resp.Data.User.DepartmentIds {
		departmentId = depId
		if depId != "0" {
			break
		}
	}
	orgName, err := getOrgnameByOrgId(client, departmentId)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(orgName)
	}

	parentOrgName, err := getParentOrgNameByOrgId(client, departmentId)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(parentOrgName)
	}

	return *resp.Data.User.Name, nil
}

// 通过组织 ID 获取组织
func getOrgnameByOrgId(client *lark.Client, orgId string) (string, error) {
	req := larkcontact.NewGetDepartmentReqBuilder().
		DepartmentId(orgId).
		UserIdType(`open_id`).
		DepartmentIdType(`open_department_id`).
		Build()

	// 发起请求
	resp, err := client.Contact.V3.Department.Get(context.Background(), req)
	if err != nil {
		fmt.Println(err)
		return "", fmt.Errorf("获取组织失败: %w", err)
	}
	if !resp.Success() {
		fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
		return "", fmt.Errorf("获取组织失败: %d", resp.CodeError.Code)
	}

	return *resp.Data.Department.Name, nil
}

// 通过组织 ID 获取父组织名称
func getParentOrgNameByOrgId(client *lark.Client, orgId string) (string, error) {
	req := larkcontact.NewParentDepartmentReqBuilder().
		UserIdType(`open_id`).
		DepartmentIdType(`open_department_id`).
		DepartmentId(orgId).
		PageSize(10).
		Build()

	resp, err := client.Contact.V3.Department.Parent(context.Background(), req)
	if err != nil {
		fmt.Println(err)
		return "", fmt.Errorf("获取父组织名称失败: %w", err)
	}
	if !resp.Success() {
		fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
		return "", fmt.Errorf("获取父组织名称失败: %d", resp.CodeError.Code)
	}

	fmt.Println(larkcore.Prettify(resp))

	orgName := ""
	for _, item := range resp.Data.Items {
		orgName += *item.Name + "."
	}
	orgName = orgName[:len(orgName)-1]

	return orgName, nil
}

func robotReply(appId string, appSecret string) error {
	/**
	 * 创建 LarkClient 对象，用于请求OpenAPI。
	 * Create LarkClient object for requesting OpenAPI
	 */
	client := lark.NewClient(appId, appSecret)

	userId, err := getUserIdByMobile(client, "13675606167")
	if err != nil {
		log.Fatalf("获取用户ID失败: %v", err)
	}
	userInfo, err := getUserInfoByUserId(client, userId)
	if err != nil {
		log.Fatalf("获取用户信息失败: %v", err)
	}
	fmt.Println(userInfo)

	/**
	 * 注册事件处理器。
	 * Register event handler.
	 */
	eventHandler := dispatcher.NewEventDispatcher("", "").
		// 用户进入与机器人的会话，发送欢迎消息
		OnP2ChatAccessEventBotP2pChatEnteredV1(func(ctx context.Context, event *larkim.P2ChatAccessEventBotP2pChatEnteredV1) error {
			fmt.Printf("[ OnP2ChatAccessEventBotP2pChatEnteredV1 access ], data: %s\n", larkcore.Prettify(event))

			// 获取用户ID 和 OpenId
			fmt.Printf("userId: %s, openId: %s\n", *event.Event.OperatorId.UserId, *event.Event.OperatorId.OpenId)
			// 获取聊天室ID
			chatId := *event.Event.ChatId
			// 构建欢迎消息
			content := larkim.NewTextMsgBuilder().
				TextLine("欢迎加入飞书聊天室，我是飞书机器人，有什么问题可以问我哦").
				Build()
			/**
			 * 使用SDK调用发送消息接口。 Use SDK to call send message interface.
			 * https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/create
			 */
			resp, err := client.Im.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
				ReceiveIdType(larkim.ReceiveIdTypeChatId). // 消息接收者的 ID 类型，设置为会话ID。 ID type of the message receiver, set to chat ID.
				Body(larkim.NewCreateMessageReqBodyBuilder().
					MsgType(larkim.MsgTypeText). // 设置消息类型为文本消息。 Set message type to text message.
					ReceiveId(chatId).           // 消息接收者的 ID 为消息发送的会话ID。 ID of the message receiver is the chat ID of the message sending.
					Content(content).
					Build()).
				Build())
			if err != nil || !resp.Success() {
				fmt.Println(err)
				fmt.Println(resp.Code, resp.Msg, resp.RequestId())
				return nil
			}

			return nil
		}).
		/**
		 * 注册接收消息事件，处理接收到的消息。
		 * Register event handler to handle received messages.
		 * https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/events/receive
		 */
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			fmt.Printf("[OnP2MessageReceiveV1 access], data: %s\n", larkcore.Prettify(event))

			// 获取用户ID 和 OpenId
			fmt.Printf("userId: %s, openId: %s\n", *event.Event.Sender.SenderId.UserId, *event.Event.Sender.SenderId.OpenId)
			/**
			 * 解析用户发送的消息。
			 * Parse the message sent by the user.
			 */
			var respContent map[string]string
			err := json.Unmarshal([]byte(*event.Event.Message.Content), &respContent)
			/**
			 * 检查消息类型是否为文本
			 * Check if the message type is text
			 */
			if err != nil || *event.Event.Message.MessageType != "text" {
				respContent = map[string]string{
					"text": "解析消息失败，请发送文本消息\nparse message failed, please send text message",
				}
			}
			/**
			 * 构建回复消息
			 * Build reply message
			 */
			content := larkim.NewTextMsgBuilder().
				TextLine("收到你发送的消息：" + respContent["text"]).
				Build()
			if *event.Event.Message.ChatType == "p2p" {
				/**
				 * 使用SDK调用发送消息接口。 Use SDK to call send message interface.
				 * https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message/create
				 */
				resp, err := client.Im.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
					ReceiveIdType(larkim.ReceiveIdTypeChatId). // 消息接收者的 ID 类型，设置为会话ID。 ID type of the message receiver, set to chat ID.
					Body(larkim.NewCreateMessageReqBodyBuilder().
						MsgType(larkim.MsgTypeText).            // 设置消息类型为文本消息。 Set message type to text message.
						ReceiveId(*event.Event.Message.ChatId). // 消息接收者的 ID 为消息发送的会话ID。 ID of the message receiver is the chat ID of the message sending.
						Content(content).
						Build()).
					Build())
				if err != nil || !resp.Success() {
					fmt.Println(err)
					fmt.Println(resp.Code, resp.Msg, resp.RequestId())
					return nil
				}
			} else {
				/**
				 * 使用SDK调用回复消息接口。 Use SDK to call send message interface.
				 * https://open.feishu.cn/document/server-docs/im-v1/message/reply
				 */
				resp, err := client.Im.Message.Reply(context.Background(), larkim.NewReplyMessageReqBuilder().
					MessageId(*event.Event.Message.MessageId).
					Body(larkim.NewReplyMessageReqBodyBuilder().
						MsgType(larkim.MsgTypeText). // 设置消息类型为文本消息。 Set message type to text message.
						Content(content).
						Build()).
					Build())
				if err != nil || !resp.Success() {
					fmt.Printf("logId: %s, error response: \n%s", resp.RequestId(), larkcore.Prettify(resp.CodeError))
					return nil
				}
			}
			return nil
		})
	/**
	 * 启动长连接，并注册事件处理器。
	 * Start long connection and register event handler.
	 */
	cli := larkws.NewClient(appId, appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelDebug),
	)
	err = cli.Start(context.Background())
	if err != nil {
		return fmt.Errorf("启动客户端失败: %w", err)
	}

	return nil
}

func main() {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}
	err = robotReply(config.Lark.AppID, config.Lark.AppSecret)
	if err != nil {
		log.Fatalf("启动机器人失败: %v", err)
	}
	fmt.Println("机器人启动成功")
}
