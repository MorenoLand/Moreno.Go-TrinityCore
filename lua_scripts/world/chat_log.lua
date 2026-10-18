-- ChatLogScript (PlayerScript chat-logging hooks) --
-- Lua port of src/server/scripts/World/chat_log.cpp (AddSC_chat_log).
--
-- The engine fires the C++ OnChat overloads as Eluna player events
-- (engine/world/chat.go): event 18 (event, player, msg, type, lang) only
-- for types with no dedicated sub-event; events 19/20/21 append the
-- routing extra (whisper receiver player, group object, guild object).
-- The hooks are cancel-only (no message rewrite), so the handlers return
-- nothing — a false return would block the chat. TC_LOG_DEBUG maps to
-- the PrintDebug global.
--
-- Ported arms (chat_log.cpp:32-140): say/emote/yell (18), whisper (19),
-- party/party-leader/raid/raid-leader/raid-warning/battleground/
-- battleground-leader (20), guild/officer (21). The group arm's
-- "leader %s" resolves GetLeaderGUID against GetMembers() (online
-- members only; "<unknown>" when the leader is offline — C++ reads the
-- DB row via Group::GetLeaderName).
--
-- NOT ported (gap classes):
-- * Channel arm (chat_log.cpp:142-155): event 22 carries only the numeric
--   channel id (chat.go:447-455); no Lua bridge resolves it to a name or
--   to the TRADE/GENERAL/CITY/LFG system-channel flags.
-- * LANG_ADDON branches: Go routes addon chat to ServerEvent 30
--   (fireAddonMessageHook), never to the player chat events, so the
--   chat.log.addon.* arms have no fire site here.

local CHAT_MSG_SAY = 1
local CHAT_MSG_PARTY = 2
local CHAT_MSG_RAID = 3
local CHAT_MSG_GUILD = 4
local CHAT_MSG_OFFICER = 5
local CHAT_MSG_YELL = 6
local CHAT_MSG_EMOTE = 10
local CHAT_MSG_RAID_LEADER = 39
local CHAT_MSG_RAID_WARNING = 40
local CHAT_MSG_BATTLEGROUND = 44
local CHAT_MSG_BATTLEGROUND_LEADER = 45
local CHAT_MSG_PARTY_LEADER = 51

local function groupLeaderName(group)
    if group == nil then
        return "<unknown>"
    end
    local leaderGuid = group:GetLeaderGUID()
    if leaderGuid == nil then
        return "<unknown>"
    end
    local members = group:GetMembers() or {}
    for _, member in ipairs(members) do
        if member:GetGUID() == leaderGuid then
            return member:GetName()
        end
    end
    return "<unknown>"
end

local function guildNameOf(guild)
    if guild == nil then
        return "<unknown>"
    end
    local name = guild:GetName()
    if name == nil then
        return "<unknown>"
    end
    return name
end

local function OnChat(event, player, msg, type, lang)
    local name = player:GetName()
    if type == CHAT_MSG_SAY then
        PrintDebug(string.format("chat.log.say: Player %s says (language %d): %s", name, lang, msg))
    elseif type == CHAT_MSG_EMOTE then
        PrintDebug(string.format("chat.log.emote: Player %s emotes: %s", name, msg))
    elseif type == CHAT_MSG_YELL then
        PrintDebug(string.format("chat.log.yell: Player %s yells (language %d): %s", name, lang, msg))
    end
end

local function OnWhisper(event, player, msg, type, lang, receiver)
    local target = "<unknown>"
    if receiver ~= nil then
        target = receiver:GetName()
    end
    PrintDebug(string.format("chat.log.whisper: Player %s tells %s: %s", player:GetName(), target, msg))
end

local function OnGroupChat(event, player, msg, type, lang, group)
    local name = player:GetName()
    if type == CHAT_MSG_PARTY then
        PrintDebug(string.format("chat.log.party: Player %s tells group with leader %s: %s", name, groupLeaderName(group), msg))
    elseif type == CHAT_MSG_PARTY_LEADER then
        PrintDebug(string.format("chat.log.party: Leader %s tells group: %s", name, msg))
    elseif type == CHAT_MSG_RAID then
        PrintDebug(string.format("chat.log.raid: Player %s tells raid with leader %s: %s", name, groupLeaderName(group), msg))
    elseif type == CHAT_MSG_RAID_LEADER then
        PrintDebug(string.format("chat.log.raid: Leader player %s tells raid: %s", name, msg))
    elseif type == CHAT_MSG_RAID_WARNING then
        PrintDebug(string.format("chat.log.raid: Leader player %s warns raid with: %s", name, msg))
    elseif type == CHAT_MSG_BATTLEGROUND then
        PrintDebug(string.format("chat.log.bg: Player %s tells battleground with leader %s: %s", name, groupLeaderName(group), msg))
    elseif type == CHAT_MSG_BATTLEGROUND_LEADER then
        PrintDebug(string.format("chat.log.bg: Leader player %s tells battleground: %s", name, msg))
    end
end

local function OnGuildChat(event, player, msg, type, lang, guild)
    local name = player:GetName()
    local gname = guildNameOf(guild)
    if type == CHAT_MSG_GUILD then
        PrintDebug(string.format("chat.log.guild: Player %s tells guild %s: %s", name, gname, msg))
    elseif type == CHAT_MSG_OFFICER then
        PrintDebug(string.format("chat.log.guild.officer: Player %s tells guild %s officers: %s", name, gname, msg))
    end
end

RegisterPlayerEvent(18, OnChat)
RegisterPlayerEvent(19, OnWhisper)
RegisterPlayerEvent(20, OnGroupChat)
RegisterPlayerEvent(21, OnGuildChat)
