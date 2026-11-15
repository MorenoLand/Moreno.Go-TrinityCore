-- Minigob Manabonk (Dalaran) --
-- Lua port of src/server/scripts/Northrend/zone_dalaran.cpp
-- (npc_minigob_manabonkAI :158-283; AddSC_dalaran registers it
-- alongside npc_mageguard_dalaran).
-- Entry: 32838 'Minigob Manabonk' (TDB-verified: creature_template
-- ScriptName 'npc_minigob_manabonk', faction 35, level 76, 1 spawn
-- guid 44457 map 571 (5877.88, 662.895, 644.53), spawntime 300).
-- Eluna creature events: 23 OnReset only. The C++ machine is a
-- pure UpdateAI EventMap chain (no combat, no gossip, no
-- JustDied/JustSummoned arms); the chain runs on CreateLuaEvent
-- timers keyed per GUID (valiant precedent), seeded from Reset.
-- C++ ctor (:176-179): setActive(true) only — no Lua bridge;
-- CreateLuaEvent timers are proximity-independent, so the machine
-- runs the same without it.
-- C++ Reset (:181-186): playerGuid clears, SetVisible(false),
-- EVENT_SELECT_TARGET in 1s.
-- C++ UpdateAI (:219-272) event chain, all delays C++-exact:
-- - EVENT_SELECT_TARGET (1s): gather Dalaran players (zone 4395,
--   not flying, not mounted, not GM); if the list is empty or
--   urand(1,100) > #players, AddObjectToRemoveList. Else
--   SetVisible(true), DoCastSelf(51347 teleport visual), pick a
--   random player, store its GUID, NearTeleportTo a point
--   frand(10,30)yd from it at a random angle (via
--   MovePositionToFirstCollision), then EVENT_LAUGH_1 in 2s.
-- - EVENT_LAUGH_1 (2s): HandleEmoteCommand(EMOTE_ONESHOT_LAUGH_
--   NO_SHEATHE 153), then EVENT_WANDER in 3s.
-- - EVENT_WANDER (3s): MoveRandom(8), then EVENT_PAUSE in 60s.
-- - EVENT_PAUSE (60s): MoveIdle, then EVENT_CAST in 2s.
-- - EVENT_CAST (2s): GetPlayer(playerGuid); if found, DoCast
--   (61839 Manabonked) + SendMailToPlayer (MailTemplate 264,
--   deliver delay irand(300,900)s); if gone,
--   AddObjectToRemoveList. Then EVENT_LAUGH_2 in 8s.
-- - EVENT_LAUGH_2 (8s): laugh emote, then EVENT_BLINK in 3s.
-- - EVENT_BLINK (3s): DoCastSelf(61995 Improved Blink), then
--   EVENT_DESPAWN in 4s.
-- - EVENT_DESPAWN (4s): AddObjectToRemoveList.
-- Despawn twin: C++ AddObjectToRemoveList marks removal at the
-- end of the map update, so the in-tick code after it still runs
-- but no later event ever fires; the Lua twin despawns and
-- returns at once (cancelTimers + Despawn, no further schedule)
-- — the observable outcome is identical.
-- Mail (C++ :207-214): MailDraft(264, true).SendMailTo with
-- sender MAIL_CREATURE entry 32838, deliverDelay irand(300,900)s.
-- The Go SendMail global takes literal subject/text, not a
-- template id; MailTemplateEntry has no Subject (DBCStructure.h:
-- 1063, Subject commented out, Body[16] only) and template mail
-- stores empty subject/body, so SendMail("", <body>) is the
-- faithful twin. Body verbatim from the wago MailTemplate CSV
-- (build 12.1.7.70892, row 264; identical on 3.4.0.47659):
-- "Hey there $n,\r\n   Remember when you got sheeped in
-- Dalaran?  Yeah, that was me, Minigob Manabonk!  ..." ($n
-- literal, client-substituted; spacing verbatim). Wand:
-- mail_loot_template 264 = item 44817 "The Mischief Maker",
-- 100%, count 1 (TDB-verified) -> SendMail items {44817, 1}.
-- Stationery defaults to 41 (MAIL_STATIONERY_DEFAULT); sender is
-- the creature's GUID low (luaSendMail writes it to the mail
-- sender column; expiry reads 30 days, the C++ normal-mail
-- rule). Spells 61839/51347/61995 are NOT in the seed Spell.dbc
-- yet, so the three casts are live-dead until a seed lands
-- (valiant 63010/65147 precedent: port first, seed after via
-- wago); the timers + mail machine run regardless.
-- Bridges: GetPlayersInRange (lua_worldobject_range.go:608,
-- default range 533.33 covers all of Dalaran from the spawn);
-- player:GetZoneId lua_player.go:173, IsFlying :1657, IsMounted
-- :1698, IsGM :1411 (the IsGameMaster twin); GetPlayerByGUID
-- (globals.go:187, dawnforge precedent, full GUID stored);
-- creature:Teleport = NearTeleportTo (lua_creature_events.go:
-- 3881, alias :3924); MoveRandom :5976; MoveIdle = StopMoving
-- :3645; SetVisible :6599; PerformEmote :4883 (emote 153 =
-- EMOTE_ONESHOT_LAUGH_NO_SHEATHE, SharedDefines.h:1995);
-- Despawn :3659 (the DespawnOrUnsummon twin); SendMail global
-- (runtime.go:1555, provider lua_globals.go:317).
-- Deviations: MovePositionToFirstCollision has no Lua bridge —
-- the teleport point is plain trig (dist*cos/sin) with no
-- collision clamp (Dalaran plaza is open ground; the point stays
-- within 30yd of the player either way).

local MINIGOB_ENTRY = 32838
local ZONE_DALARAN = 4395
local SPELL_MANABONKED = 61839
local SPELL_TELEPORT_VISUAL = 51347
local SPELL_IMPROVED_BLINK = 61995
local EMOTE_LAUGH = 153 -- EMOTE_ONESHOT_LAUGH_NO_SHEATHE, SharedDefines.h:1995
local MAIL_WAND_ENTRY = 44817 -- "The Mischief Maker", mail_loot_template 264, 100%, x1
local MAIL_BODY = "Hey there $n,\r\n   Remember when you got sheeped in Dalaran?  Yeah, that was me, Minigob Manabonk!  I just wanted to send you a letter to let you know who sheeped you.  Also, because you were such a good sport about it I'm sending you a wand of my own.  Now you can \"Manabonk\" your friends.  When you do, tell them \"Minigob Manabonk sent you.\"\r\n\r\n- Minigob Manabonk"

local timers = {}
local state = {}

-- Forward declarations: the chain handlers reference each other
-- in CreateLuaEvent closures; a later `local function` would bind the
-- closure to a nil global (malown time-bomb). Plain assignments below.
local onSelectTarget, onLaugh1, onWander, onPause, onCast, onLaugh2, onBlink, onDespawn

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_SELECT_TARGET (:230-249): gather Dalaran players,
-- despawn on empty list or failed 1%-per-player roll, else
-- appear + teleport visual + NearTeleportTo near a random pick.
onSelectTarget = function(creature, guid)
    local players = {}
    for _, player in ipairs(creature:GetPlayersInRange()) do
        if player:GetZoneId() == ZONE_DALARAN and not player:IsFlying() and not player:IsMounted() and not player:IsGM() then
            players[#players + 1] = player
        end
    end
    if #players == 0 or math.random(1, 100) > #players then
        cancelTimers(guid)
        state[guid] = nil
        creature:Despawn()
        return
    end
    creature:SetVisible(true)
    creature:CastSpell(creature, SPELL_TELEPORT_VISUAL)
    local player = players[math.random(#players)]
    state[guid] = { playerGuid = player:GetGUID() }
    local dist = 10 + math.random() * 20
    local angle = math.random() * 2 * math.pi
    creature:Teleport(player:GetX() + dist * math.cos(angle), player:GetY() + dist * math.sin(angle), player:GetZ(), player:GetO())
    schedule(guid, "laugh1", 2000, function() onLaugh1(creature, guid) end)
end

-- C++ EVENT_LAUGH_1 (:250-253): laugh, then wander in 3s.
onLaugh1 = function(creature, guid)
    creature:PerformEmote(EMOTE_LAUGH)
    schedule(guid, "wander", 3000, function() onWander(creature, guid) end)
end

-- C++ EVENT_WANDER (:254-257): MoveRandom(8), then pause in 60s.
onWander = function(creature, guid)
    creature:MoveRandom(8)
    schedule(guid, "pause", 60000, function() onPause(creature, guid) end)
end

-- C++ EVENT_PAUSE (:258-261): MoveIdle, then cast in 2s.
onPause = function(creature, guid)
    creature:MoveIdle()
    schedule(guid, "cast", 2000, function() onCast(creature, guid) end)
end

-- C++ EVENT_CAST (:262-270): DoCast(61839) + SendMail to the
-- stored player; despawn if the player is gone.
onCast = function(creature, guid)
    local st = state[guid]
    local player = st and GetPlayerByGUID(st.playerGuid) or nil
    if player then
        creature:CastSpell(player, SPELL_MANABONKED)
        SendMail("", MAIL_BODY, player:GetGUIDLow(), creature:GetGUIDLow(), nil, math.random(300, 900), 0, 0, MAIL_WAND_ENTRY, 1)
    else
        cancelTimers(guid)
        state[guid] = nil
        creature:Despawn()
        return
    end
    schedule(guid, "laugh2", 8000, function() onLaugh2(creature, guid) end)
end

-- C++ EVENT_LAUGH_2 (:271-274): laugh, then blink in 3s.
onLaugh2 = function(creature, guid)
    creature:PerformEmote(EMOTE_LAUGH)
    schedule(guid, "blink", 3000, function() onBlink(creature, guid) end)
end

-- C++ EVENT_BLINK (:275-278): DoCastSelf(61995), despawn in 4s.
onBlink = function(creature, guid)
    creature:CastSpell(creature, SPELL_IMPROVED_BLINK)
    schedule(guid, "despawn", 4000, function() onDespawn(creature, guid) end)
end

-- C++ EVENT_DESPAWN (:279-281): AddObjectToRemoveList.
onDespawn = function(creature, guid)
    cancelTimers(guid)
    state[guid] = nil
    creature:Despawn()
end

-- C++ Reset (:181-186): clear the stored player, hide, seed
-- the select-target event in 1s. (fireCreatureSpawned fires
-- ev23 before ev5, so this also runs at spawn, as C++ Reset
-- does on a fresh AI.)
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    creature:SetVisible(false)
    schedule(guid, "select_target", 1000, function() onSelectTarget(creature, guid) end)
end

RegisterCreatureEvent(MINIGOB_ENTRY, 23, onReset)
