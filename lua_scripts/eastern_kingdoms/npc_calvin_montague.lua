-- Calvin Montague (Tirisfal Glades zone script)
-- Lua port of src/server/scripts/EasternKingdoms/
-- zone_tirisfal_glades.cpp (npc_calvin_montagueAI — ScriptedAI,
-- registered by AddSC_tirisfal_glades in
-- eastern_kingdoms_script_loader.cpp (declaration line 189, call
-- line 367, after AddSC_undercity).
-- Entry: the C++ sources carry NO NPC_ constant for this script
-- (nothing in the file's own enums — the script binds by DB
-- ScriptName), so the entry is cited from
-- classic.wowhead.com/npc=6784/calvin-montague (spectral-citizen
-- precedent): Calvin Montague, level 5, Tirisfal Glades.
-- Quest 590 is "A Rogue's Deal" (start + end: Calvin Montague,
-- "Defeat Calvin Montague in Deathknell") — NOT "The Family Crypt"
-- (quest 408, Magistrate Sevren), per the Wowhead quest page.
-- Eluna creature events: 5 OnSpawn, 9 OnDamageTaken, 23 OnReset,
-- 31 OnQuestAccept. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. JustEngagedWith is empty in C++.
-- Verifiable numbers (file's own Calvin enum): SAY_COMPLETE = 0,
-- SPELL_DRINK = 7737, QUEST_590 = 590.
-- Ported arms (C++-exact):
-- - OnQuestAccept(590): store the player GUID, SetFaction(
--   FACTION_ENEMY 168, SharedDefines.h:241), RemoveFlag(
--   UNIT_NPC_FLAGS 82, UNIT_NPC_FLAG_QUESTGIVER 2), AttackStart.
-- - DamageTaken (zone_tirisfal_glades.cpp:97-105): damage >
--   health OR HealthBelowPctDamaged(15, damage) -> damage = 0
--   (ev9 rewrite, `return false, 0` malchezaar precedent),
--   CombatStop(true), then the 3s/2s/5s/12s/3s event chain:
--   PerformEmote(EMOTE_ONESHOT_RUDE 14) -> Talk(SAY_COMPLETE 0)
--   -> player AreaExploredOrEventHappens(590) + DoCastSelf(
--   SPELL_DRINK 7737) -> SetFlag(UNIT_NPC_FLAGS,
--   UNIT_NPC_FLAG_QUESTGIVER) -> SetStandState(STAND 0).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - EnterEvadeMode (DamageTaken :101): no bridge (standing). The
--   duel happens at Calvin's spawn point, so the walk-home leg is
--   unobservable; CombatStop(true) + the chain cover the rest.
-- - RestoreFaction (Reset :84): no bridge. Replicated with the
--   GetFaction/SetFaction pair: the native faction is read on
--   OnSpawn and restored in DamageTaken (C++ restores it in Reset,
--   which EnterEvadeMode fires immediately) and on OnReset.
-- - SetImmuneToPC (Reset :85-86, OnQuestAccept :137): the Go
--   engine has no immunity-to-PC model (zero hits outside
--   comments), so both arms are vacuous here. Their observable
--   effect is replicated instead: while no quest player is stored
--   (before quest accept, or after the surrender latch) DamageTaken
--   returns `false, 0`, so attacks do nothing — exactly as C++
--   immunity behaves; the surrender chain only arms once a quest
--   player exists.

local ENTRY_NPC_CALVIN_MONTAGUE = 6784

local QUEST_A_ROGUES_DEAL = 590
local SPELL_DRINK = 7737

local SAY_COMPLETE = 0

local FACTION_ENEMY = 168

local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_QUESTGIVER = 2

local EMOTE_ONESHOT_RUDE = 14

local UNIT_STAND_STATE_STAND = 0

local HEALTH_PCT_SURRENDER = 15

local timers = {}
local playerGuid = {}
local nativeFaction = {}
local surrendered = {}

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

-- C++ Reset (zone_tirisfal_glades.cpp:82-88): RestoreFaction +
-- SetImmuneToPC(true). The faction restore rides GetFaction (read
-- on spawn) + SetFaction; the immunity arm is vacuous (no engine
-- model). Cancelling the chain + clearing the latch matches the
-- in-tree reset idiom (spectral-citizen); the C++ EventMap is not
-- cleared in Reset, but the chain can only be pending while
-- surrendered, and a reset while surrendered only fires at respawn.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    playerGuid[guid] = nil
    surrendered[guid] = nil
    if nativeFaction[guid] then
        creature:SetFaction(nativeFaction[guid])
    end
end

local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    playerGuid[guid] = nil
    surrendered[guid] = nil
end

-- C++ OnSpawn: record the template faction before any quest arm
-- can change it, so the Reset/RestoreFaction substitute has the
-- C++-exact value.
local function onSpawn(_, creature)
    local guid = creature:GetGUIDLow()
    nativeFaction[guid] = creature:GetFaction()
end

-- C++ OnQuestAccept (zone_tirisfal_glades.cpp:128-141, C++ order):
-- store the player GUID, SetFaction(FACTION_ENEMY),
-- SetImmuneToPC(false) [vacuous, no engine model], AttackStart,
-- RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER).
local function onQuestAccept(_, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_A_ROGUES_DEAL then
        return
    end
    if not player then
        return
    end
    local guid = creature:GetGUIDLow()
    playerGuid[guid] = player:GetGUID()
    creature:SetFaction(FACTION_ENEMY)
    creature:AttackStart(player)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
end

-- C++ DamageTaken (zone_tirisfal_glades.cpp:90-106): lethal damage
-- or HealthBelowPctDamaged(15, damage) -> damage = 0, CombatStop,
-- EnterEvadeMode [no bridge], schedule EVENT_EMOTE_RUDE 3s. This
-- is a DamageTaken arm, so it DOES fire on the killing blow
-- (nefarian rule: only UpdateAI arms skip lethal hits) — no
-- postDamage > 0 guard. The surrender latch + the no-quest-player
-- zero-damage return replicate the C++ immunity arms (see header).
local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUIDLow()
    if surrendered[guid] or not playerGuid[guid] then
        return false, 0
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = health - damage
    if damage > health or postDamage * 100 < HEALTH_PCT_SURRENDER * maxHealth then
        surrendered[guid] = true
        creature:CombatStop(true)
        -- C++ Reset restores the faction immediately via the evade;
        -- the evade itself has no bridge, so restore it here.
        if nativeFaction[guid] then
            creature:SetFaction(nativeFaction[guid])
        end
        schedule(guid, "emote", 3000, function()
            creature:PerformEmote(EMOTE_ONESHOT_RUDE)
            schedule(guid, "talk", 2000, function()
                creature:Talk(SAY_COMPLETE)
                schedule(guid, "drink", 5000, function()
                    local player = GetPlayerByGUID(playerGuid[guid])
                    if player then
                        player:AreaExploredOrEventHappens(QUEST_A_ROGUES_DEAL)
                    end
                    playerGuid[guid] = nil
                    creature:CastSpell(creature, SPELL_DRINK)
                    schedule(guid, "flag", 12000, function()
                        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
                        schedule(guid, "stand", 3000, function()
                            creature:SetStandState(UNIT_STAND_STATE_STAND)
                        end)
                    end)
                end)
            end)
        end)
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_NPC_CALVIN_MONTAGUE, 5, onSpawn)
RegisterCreatureEvent(ENTRY_NPC_CALVIN_MONTAGUE, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_NPC_CALVIN_MONTAGUE, 23, onReset)
RegisterCreatureEvent(ENTRY_NPC_CALVIN_MONTAGUE, 4, onDied)
RegisterCreatureEvent(ENTRY_NPC_CALVIN_MONTAGUE, 31, onQuestAccept)
