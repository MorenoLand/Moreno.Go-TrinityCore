-- npc_death_knight_initiate (The Scarlet Enclave, Acherus DK intro,
-- Quest 12733 "Death Challenge") -- Lua port of the
-- npc_death_knight_initiate class in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter1.cpp
-- (class npc_death_knight_initiate line 540; AI struct
-- npc_death_knight_initiateAI line 545 : CombatAI, no spells
-- attached -- "@todo spells" -- so CombatAI::UpdateAI is melee-only
-- and the Go engine-driven combat tick covers it).
-- Entries (TDB 26091 creature_template, name + spawn + gossip-menu
-- evidence -- the dump's ScriptName column is unpopulated, 16/29923
-- rows, so no ScriptName binding is verifiable from it): 28390,
-- 28391, 28392, 28393, 28394 (lvl 55, faction 1770, no gossip flag,
-- map 609 spawns) and 28406 (lvl 55, faction 2084, npcflag 1 GOSSIP,
-- gossip_menu_id 9765, 16 map-609 spawns). The duel flow only ever
-- targets 28406 (the gossip arm casts SPELL_DUEL on `me`); the other
-- five register the same combat arms (harmless: no latch without a
-- SPELL_DUEL hit, damage passes through unchanged).
-- Gossip menu 9765 option 0: "I challenge you, death knight!"
-- (gossip_menu_option OptionText, broadcast 29255), icon 0, sender
-- MAIN, action GOSSIP_ACTION_INFO_DEF (1000).
-- Verifiable numbers (file's own Spells_DKI / Says_VBM / Misc_VBN
-- enums, lines 521-538): SPELL_DUEL = 52996 / SPELL_DUEL_VICTORY =
-- 52994 / SPELL_DUEL_FLAG = 52991 / SPELL_GROVEL = 7267 /
-- SAY_DUEL = 0 / QUEST_DEATH_CHALLENGE = 12733.
-- FACTION_UNDEAD_SCOURGE_2 = 2068 (SharedDefines.h:263);
-- UNIT_FLAG_SWIMMING = 0x8000 (UnitDefines.h:139);
-- UNIT_FLAG_IMMUNE_TO_PC = 0x100 on UNIT_FIELD_FLAGS (59).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (returns false, newDamage -- midnight precedent),
-- 14 OnHitBySpell, 23 OnReset; gossip 1 OnGossipHello, 2
-- OnGossipSelect (zulaman / hummel precedents). Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (chapter1.cpp:540-695):
-- - OnGossipHello (chapter1.cpp:671-685): quest 12733 incomplete +
--   IsFullHealth + player health >= 10% + neither in combat ->
--   GossipMenuAddItem(0, "I challenge you, death knight!", 1, 1000)
--   + GossipSendMenu(GetGossipTextId). The hook vetoes the default
--   menu when it returns nothing (gossip.go hello arm), so gated-out
--   players see nothing -- C++-exact (C++ returns true with no menu).
-- - OnGossipSelect (chapter1.cpp:648-669): sender MAIN + action
--   1000 -> GossipClearMenu + GossipComplete (C++ Clear/CloseGossip
--   MenuFor, same order -- before the combat/duel gates) -> SetImmune
--   ToPC(false) (RemoveFlag 59, 0x100) + RemoveFlag(SWIMMING) ->
--   player:CastSpell(me, SPELL_DUEL 52996, false) +
--   player:CastSpell(player, SPELL_DUEL_FLAG 52991, true). Returns
--   nothing (veto: C++ returns true, the AI arm IS the port).
-- - SpellHit (chapter1.cpp:573-582): SPELL_DUEL hit latches the duel
--   (dueler GUID + Talk SAY_DUEL 0 whispered to the caster) -- ev14
--   args are (event, creature, caster, spellId), attumen precedent.
-- - Duel timer (chapter1.cpp:609-623): 5s out-of-combat wait ->
--   SetFaction(2068) + AttackStart(dueler) via GetPlayerByGUID +
--   EngageWithTarget. The timer only ticks while out of combat, so a
--   fire that finds a victim re-arms 1s (faithful to the UpdateAI
--   gate); it is NOT cancelled on leave-combat.
-- - DamageTaken (chapter1.cpp:584-607): while the duel runs, damage
--   from anyone but the dueler (or the dueler's pet, owner-GUID test)
--   is zeroed ("other players cannot help"); a lethal hit from the
--   dueler is zeroed once and runs the victory leg: attacker
--   RemoveGameObject(52991) + AttackStop, creature CastSpell
--   (attacker, SPELL_DUEL_VICTORY 52994, true), self-cast GROVEL
--   7267, RestoreFaction. RestoreFaction has no bridge -- it is the
--   template-faction SetFaction twin (Unit.cpp:12027-12044: normal
--   creature -> cinfo->faction), 1770 / 2084 per entry below.
-- - UpdateAI lose arm (chapter1.cpp:626-633): after the victory, if
--   the GROVEL aura is gone the creature evades -- 1s poll (the Go
--   model treats 7267 as a visual-only kneel, combat.go:4025, so no
--   aura ever lands and the first poll evades, ~1s of kneel).
-- - UpdateAI beg arm (chapter1.cpp:634-641): while dueling, victim
--   below 10% health casts GROVEL on itself, removes the duel flag
--   and the creature evades -- 500ms combat poll (no DamageDealt
--   hook exists in Go; poll vs per-tick is the documented
--   approximation). The poll starts on enter-combat only while a
--   duel runs.
-- - Reset (chapter1.cpp:566-571): clear duel state + RestoreFaction
--   (template-faction twin) + SetFlag(SWIMMING); CombatAI::Reset is
--   a no-op (no spells attached).
-- Deviations from C++: no IsControlledByPlayer bridge -- the
-- GUID/owner-GUID test matches C++ for every player/pet attacker and
-- only diverges for a hypothetical wild-creature attacker mid-duel
-- (damage zeroed instead of passing through; unreachable in the
-- Acherus hub). SPELL_DUEL_FLAG's gameobject-summon leg is
-- unmodeled (no Go handler verified for its summon effect): the
-- 52991 seed row is an INFERRED DUMMY self-target row, so
-- RemoveGameObject rides as a no-op. The 5s duel timer and the two
-- UpdateAI polls are CreateLuaEvent approximations of the per-tick
-- C++ UpdateAI arms. SPELL_DUEL 52996 / DUEL_VICTORY 52994 /
-- GROVEL 7267 seed rows are INFERRED (tools/spelldbc_seed/main.go):
-- DUMMY effects, TargetA 25 (52996/52994) / 1 (7267 self), range
-- 93005 (0-20 yd) for the targeted pair; replace with real DBC rows
-- when client data ships.
local SPELL_DUEL = 52996
local SPELL_DUEL_VICTORY = 52994
local SPELL_DUEL_FLAG = 52991
local SPELL_GROVEL = 7267

local SAY_DUEL = 0
local QUEST_DEATH_CHALLENGE = 12733
local QUEST_STATUS_INCOMPLETE = 3

local FACTION_UNDEAD_SCOURGE_2 = 2068
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ICON_CHAT = 0
local GOSSIP_CHALLENGE = "I challenge you, death knight!"

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local UNIT_FLAG_SWIMMING = 0x8000

local ENTRIES = { 28390, 28391, 28392, 28393, 28394, 28406 }
local DUEL_ENTRY = 28406

-- Template factions (TDB 26091 creature_template): 28390-28394 ship
-- 1770, 28406 ships 2084. RestoreFaction's template-faction twin.
local TEMPLATE_FACTION = {
    [28390] = 1770, [28391] = 1770, [28392] = 1770,
    [28393] = 1770, [28394] = 1770, [28406] = 2084,
}

local timers = {}
local state = {}

local function cancelTimers(guid, keepDuelTimer)
    local per = timers[guid]
    if per then
        for key, id in pairs(per) do
            if not (keepDuelTimer and key == "duelTimer") then
                RemoveEventById(id)
                per[key] = nil
            end
        end
        if next(per) == nil then
            timers[guid] = nil
        end
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

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { duel = false, lose = false, duelerGUID = 0 }
        state[guid] = st
    end
    return st
end

local function clearState(guid)
    state[guid] = nil
end

local function restoreFaction(creature)
    local faction = TEMPLATE_FACTION[creature:GetEntry()]
    if faction then
        creature:SetFaction(faction)
    end
end

-- C++ UpdateAI duel timer (chapter1.cpp:609-623): 5s out-of-combat
-- wait -> SetFaction(2068) + AttackStart(dueler).
local function onDuelTimer(creature, guid)
    local st = state[guid]
    if not st or not st.duel then
        return
    end
    if creature:GetVictim() ~= nil then
        schedule(guid, "duelTimer", 1000, function() onDuelTimer(creature, guid) end)
        return
    end
    creature:SetFaction(FACTION_UNDEAD_SCOURGE_2)
    local dueler = creature:GetPlayerByGUID(st.duelerGUID)
    if dueler then
        creature:EngageWithTarget(dueler)
    end
end

-- C++ UpdateAI beg arm (chapter1.cpp:634-641): victim below 10%
-- casts GROVEL on itself, removes the flag, creature evades.
local function onCombatPoll(creature, guid)
    local st = state[guid]
    if not st or not st.duel or st.lose then
        return
    end
    local victim = creature:GetVictim()
    if victim == nil then
        return
    end
    if victim:HealthBelowPct(10) then
        victim:CastSpell(victim, SPELL_GROVEL, true)
        victim:RemoveGameObject(SPELL_DUEL_FLAG, true)
        creature:EnterEvadeMode()
        return
    end
    schedule(guid, "combatPoll", 500, function() onCombatPoll(creature, guid) end)
end

-- C++ UpdateAI lose arm (chapter1.cpp:626-633): once the GROVEL aura
-- is gone the creature evades.
local function onLosePoll(creature, guid)
    local st = state[guid]
    if not st or not st.lose then
        return
    end
    if creature:GetVictim() == nil then
        return
    end
    if not creature:HasAura(SPELL_GROVEL) then
        creature:EnterEvadeMode()
        return
    end
    schedule(guid, "losePoll", 1000, function() onLosePoll(creature, guid) end)
end

-- C++ OnGossipHello (chapter1.cpp:671-685).
local function onGossipHello(_, player, creature)
    if player:GetQuestStatus(QUEST_DEATH_CHALLENGE) == QUEST_STATUS_INCOMPLETE
        and creature:IsFullHealth()
        and not player:HealthBelowPct(10)
        and not player:IsInCombat()
        and not creature:IsInCombat() then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_CHALLENGE, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF)
        player:GossipSendMenu(player:GetGossipTextId(creature), creature)
    end
end

-- C++ OnGossipSelect (chapter1.cpp:648-669).
local function onGossipSelect(_, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN or action ~= GOSSIP_ACTION_INFO_DEF then
        return
    end
    player:GossipClearMenu()
    player:GossipComplete()
    if player:IsInCombat() or creature:IsInCombat() then
        return
    end
    local st = state[creature:GetGUIDLow()]
    if st and st.duel then
        return
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_SWIMMING)
    player:CastSpell(creature, SPELL_DUEL, false)
    player:CastSpell(player, SPELL_DUEL_FLAG, true)
end

-- C++ SpellHit (chapter1.cpp:573-582).
local function onSpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_DUEL then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = getState(guid)
    if st.duel or caster == nil then
        return
    end
    st.duel = true
    st.duelerGUID = caster:GetGUID()
    creature:Talk(SAY_DUEL, caster)
    schedule(guid, "duelTimer", 5000, function() onDuelTimer(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    local st = state[guid]
    if st and st.duel and not st.lose then
        schedule(guid, "combatPoll", 500, function() onCombatPoll(creature, guid) end)
    end
end

local function onLeaveCombat(_, creature)
    cancelTimers(creature:GetGUIDLow(), true)
end

-- C++ DamageTaken (chapter1.cpp:584-607).
local function onDamageTaken(_, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local st = state[guid]
    if not st or not st.duel then
        return false, damage
    end
    if attacker == nil then
        return false, damage
    end
    local attackerGUID = attacker:GetGUID()
    local ownerGUID = attacker:GetOwnerGUID()
    if attackerGUID ~= st.duelerGUID and ownerGUID ~= st.duelerGUID then
        return false, 0
    end
    if damage >= creature:GetHealth() then
        if not st.lose then
            st.lose = true
            attacker:RemoveGameObject(SPELL_DUEL_FLAG, true)
            attacker:AttackStop()
            creature:CastSpell(attacker, SPELL_DUEL_VICTORY, true)
            creature:CastSpell(creature, SPELL_GROVEL, true)
            restoreFaction(creature)
            schedule(guid, "losePoll", 1000, function() onLosePoll(creature, guid) end)
        end
        return false, 0
    end
    return false, damage
end

local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid, false)
    clearState(guid)
end

-- C++ Reset (chapter1.cpp:566-571).
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid, false)
    clearState(guid)
    restoreFaction(creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_SWIMMING)
end

for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 9, onDamageTaken)
    RegisterCreatureEvent(entry, 14, onSpellHit)
    RegisterCreatureEvent(entry, 23, onReset)
    RegisterCreatureGossipEvent(entry, 1, onGossipHello)
    RegisterCreatureGossipEvent(entry, 2, onGossipSelect)
end
