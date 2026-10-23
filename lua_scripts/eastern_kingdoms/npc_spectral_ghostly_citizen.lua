-- Spectral / Ghostly Citizen (Stratholme zone trash)
-- Lua port of src/server/scripts/EasternKingdoms/Stratholme/
-- stratholme.cpp (npc_spectral_ghostly_citizenAI — ScriptedAI via
-- GetStratholmeAI -> GetInstanceAI; registered by AddSC_stratholme in
-- eastern_kingdoms_script_loader.cpp (declaration line 138, call
-- line 316, after AddSC_instance_stratholme). The Stratholme block
-- is CLOSED with this unit.
-- Entries: the C++ sources carry NO NPC_ constant for this script
-- (nothing in stratholme.h or the file's own enums — the script binds
-- by DB ScriptName), so both entries are cited from
-- classic.wowhead.com/npc=10384/spectral-citizen and
-- classic.wowhead.com/npc=10385/ghostly-citizen (baroness_anastari
-- precedent). A Stratholme issue thread confirms the script's two
-- spells as these mobs' abilities (16333 / 16336).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Verifiable numbers (file's own GhostlyCitizenSpells enum):
-- SPELL_HAUNTING_PHANTOM = 16336, SPELL_DEBILITATING_TOUCH = 16333.
-- Ported arms (C++ UpdateAI combat machine — C++-exact):
-- - Haunting Phantom: 8000 init -> 11000 re-arm, non-triggered
--   DoCast(SelectTarget(Random, 0)). C++ DefaultTargetSelector with
--   dist = 0 means "ignored" = unlimited (UnitAI.h:63), so the pick
--   is a random alive player on map+instance with no distance bound
--   (baron_rivendare precedent); nil pick casts nothing but the
--   re-arm is unconditional, like C++ (the C++ arm re-arms even when
--   SelectTarget returns null).
-- - Debilitating Touch: 2000 init -> 7000 re-arm, same target/pick
--   rules as Haunting Phantom.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The Egan-blaster Tagged latch (SpellHit SPELL_EGAN_BLASTER =
--   17368 from the file's RestlessSoul enum; event 14 fires in the
--   Go engine but the latch's only outlets are a 5s KillSelf leg
--   (no kill bridge) and a JustDied leg that summons NPC_RESTLESS =
--   11122 via 4x urand(1, i) == 1 DoSummon(20f, 10min) (no summon
--   bridge, herod precedent) — tracking the latch alone would be a
--   stub, so it is not registered).
-- - ReceiveEmote dance -> EnterEvadeMode: no EnterEvadeMode bridge,
--   documented-only. The rude arm IS ported (see onReceiveEmote):
--   IsWithinDist is in the object.go generic fallback so the 5yd gate
--   needs no player X/Y; the wave/bow/kiss visual arms ride
--   creature:PerformEmote.
-- - GetStratholmeAI -> GetInstanceAI leg (instance-model blocked,
--   standing).

local ENTRY_SPECTRAL_CITIZEN = 10384
local ENTRY_GHOSTLY_CITIZEN = 10385

local SPELL_HAUNTING_PHANTOM = 16336
local SPELL_DEBILITATING_TOUCH = 16333

-- C++ ReceiveEmote (stratholme.cpp:297-320, npc_spectral_ghostly_citizenAI):
-- event 8 fires in the Go engine (emote.go:217) with
-- (event, creature, player, textEmote). Text-emote IDs from
-- SharedDefines.h, oneshot IDs from the same file's Emote enum;
-- creature:PerformEmote == Unit::HandleEmoteCommand (handler twin,
-- lua_creature_events.go:325; event 8 fires the handler creature via
-- fireCreatureLuaEvent, so the session-bound twin at lua_objects.go:375
-- alone nil-calls here).
local SPELL_SLAP = 6754

local TEXT_EMOTE_BOW = 17
local TEXT_EMOTE_KISS = 58
local TEXT_EMOTE_RUDE = 77
local TEXT_EMOTE_WAVE = 101

local EMOTE_ONESHOT_BOW = 2
local EMOTE_ONESHOT_WAVE = 3
local EMOTE_ONESHOT_RUDE = 14
local EMOTE_ONESHOT_FLEX = 23

-- C++ stratholme.cpp:297-320: rude -> DoCast(player, SPELL_SLAP, false)
-- within 5yd (IsWithinDistInMap; IsWithinDist is the object.go fallback
-- twin), else HandleEmoteCommand(EMOTE_ONESHOT_RUDE) == PerformEmote per
-- the handler twin; dance -> EnterEvadeMode stays out (no bridge).
local function onReceiveEmote(_, creature, player, textEmote)
    if textEmote == TEXT_EMOTE_WAVE then
        creature:PerformEmote(EMOTE_ONESHOT_WAVE)
    elseif textEmote == TEXT_EMOTE_BOW then
        creature:PerformEmote(EMOTE_ONESHOT_BOW)
    elseif textEmote == TEXT_EMOTE_KISS then
        creature:PerformEmote(EMOTE_ONESHOT_FLEX)
    elseif textEmote == TEXT_EMOTE_RUDE then
        if player and creature:IsWithinDist(player, 5) then
            creature:CastSpell(player, SPELL_SLAP)
        else
            creature:PerformEmote(EMOTE_ONESHOT_RUDE)
        end
    end
end

local timers = {}

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

-- C++ SelectTarget(Random, 0): random alive player on the creature's
-- map+instance, no distance bound (dist 0 = ignored = unlimited,
-- baron_rivendare precedent).
local function randomPlayerUnbounded(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ EVENT-less timer: HauntingTimer 8000 init, 11000 re-arm;
-- non-triggered DoCast(target, 16336). Nil pick keeps the schedule.
local function onHauntingPhantom(creature, guid)
    local target = randomPlayerUnbounded(creature)
    if target then
        creature:CastSpell(target, SPELL_HAUNTING_PHANTOM)
    end
    schedule(guid, "haunting", 11000, function()
        onHauntingPhantom(creature, guid)
    end)
end

-- TouchTimer 2000 init, 7000 re-arm; non-triggered DoCast(16333).
local function onDebilitatingTouch(creature, guid)
    local target = randomPlayerUnbounded(creature)
    if target then
        creature:CastSpell(target, SPELL_DEBILITATING_TOUCH)
    end
    schedule(guid, "touch", 7000, function()
        onDebilitatingTouch(creature, guid)
    end)
end

-- C++ Reset -> Initialize(): both timers back to C++ initials.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "haunting", 8000, function()
        onHauntingPhantom(creature, guid)
    end)
    schedule(guid, "touch", 2000, function()
        onDebilitatingTouch(creature, guid)
    end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({ ENTRY_SPECTRAL_CITIZEN, ENTRY_GHOSTLY_CITIZEN }) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onCombatEnd)
    RegisterCreatureEvent(entry, 4, onCombatEnd)
    RegisterCreatureEvent(entry, 8, onReceiveEmote)
    RegisterCreatureEvent(entry, 23, onReset)
end
