package protocol

func BuildChatMessage(chatType uint8, language uint32, senderGUID, receiverGUID uint64, message, channel string) []byte {
	return BuildChatMessageWithOptions(chatType, language, senderGUID, receiverGUID, message, channel, false, "", 0)
}

func BuildChatMessageWithOptions(chatType uint8, language uint32, senderGUID, receiverGUID uint64, message, channel string, gmMessage bool, senderName string, chatTag uint8) []byte {
	packet := NewBuffer(48 + len(message) + len(channel))
	packet.WriteU8(chatType)
	packet.WriteU32(language)
	packet.WriteU64(senderGUID)
	packet.WriteU32(0)
	if gmMessage {
		packet.WriteU32(uint32(len(senderName) + 1))
		packet.WriteCString(senderName)
	}
	if chatType == 0x11 {
		packet.WriteCString(channel)
	}
	packet.WriteU64(receiverGUID)
	packet.WriteU32(uint32(len(message) + 1))
	packet.WriteCString(message)
	packet.WriteU8(chatTag)
	return packet.Bytes()
}

func BuildSystemChatMessage(message string) []byte {
	return BuildChatMessage(0, 0, 0, 0, message, "")
}

// BuildChatServerMessage builds an SMSG_CHAT_SERVER_MESSAGE payload:
// int32 MessageID followed by the string parameter, mirroring
// WorldPackets::Chat::ChatServerMessage::Write (ChatPackets.cpp:33-39).
// World::SendServerMessage (World.cpp:3074) only fills StringParam when
// MessageID <= SERVER_MSG_STRING (World.h:45-52; SERVER_MSG_STRING = 3).
func BuildChatServerMessage(messageID int32, stringParam string) []byte {
	packet := NewBuffer(4 + len(stringParam) + 1)
	packet.WriteI32(messageID)
	packet.WriteCString(stringParam)
	return packet.Bytes()
}

// BuildMonsterChatMessage builds an SMSG_MESSAGECHAT payload for monster speech
// (say, yell, whisper, emote, boss emote).
// Reference: TrinityCore Chat.cpp:203-220.
func BuildMonsterChatMessage(chatType uint8, language uint32, senderGUID uint64, senderName, message string) []byte {
	packet := NewBuffer(48 + len(senderName) + len(message))
	packet.WriteU8(chatType)
	packet.WriteU32(language)
	packet.WriteU64(senderGUID)
	packet.WriteU32(0)
	packet.WriteU32(uint32(len(senderName) + 1))
	packet.WriteCString(senderName)
	packet.WriteU64(0)
	packet.WriteU32(uint32(len(message) + 1))
	packet.WriteCString(message)
	packet.WriteU8(0)
	return packet.Bytes()
}
