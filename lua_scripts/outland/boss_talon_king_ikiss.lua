-- Talon King Ikiss (Sethekk Halls, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/SethekkHalls/boss_talon_king_ikiss.cpp
-- (1 CreatureScript — boss_talon_king_ikiss (BossAI via
-- GetSethekkHallsAI, DATA_TALON_KING_IKISS = 1); registered from
-- inside AddSC_boss_talon_king_ikiss(); loader decl/call per
-- outland_script_loader.cpp — the "Auchindoun - Sethekk Halls"
-- sub-block — verified from the loader this run).
-- Entry: 18473 — DB-side (no NPC_TALON_KING_IKISS constant in
-- sethekk_halls.h; ScriptName bindings are DB-side as usual).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_talon_king_ikiss" hits boss_talon_king_ikiss.cpp (+ the
-- loader decl/call lines) only. No boss_talon_king_ikiss lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset,
-- 27 OnMoveInLOS.
-- Ported arms (C++-exact for all modeled arms):
-- boss_talon_king_ikissAI::Reset — clears the Intro and ManaShield
-- latches (event 23).
-- boss_talon_king_ikissAI::MoveInLineOfSight — first player within
-- 100.0f: Intro latch + Talk(SAY_INTRO 0) (event 27; the handler
-- must NOT return true — that vetoes the aggro engage —
-- lua_creature_events.go:509-524).
-- boss_talon_king_ikissAI::JustEngagedWith — Talk(SAY_AGGRO 1) +
-- schedules EVENT_ARCANE_VOLLEY (5s), EVENT_POLYMORPH (8s),
-- EVENT_BLINK (35s), heroic-only EVENT_SLOW (15-30s rand) (event 1;
-- auriaya precedent for the Talk).
-- EVENT_POLYMORPH — heroic: SelectTarget(Random, 0) →
-- DoCast(target, SPELL_POLYMORPH 38245); normal:
-- SelectTarget(MaxThreat, 1) (SECOND on threat, tank excluded;
-- playerOnly defaults FALSE per UnitAI.h:170, so pets/totems eligible) →
-- DoCast(target, SPELL_POLYMORPH); re-arm randtime(15s,17.5s)
-- (C++ 15s, 17500ms — millisecond granularity, not whole seconds).
-- GetAITargets is threat-SORTED (lua_creature_events.go:345-359),
-- so the netherspite/curator offset-1 precedent applies; nil-target
-- falls through with no cast (C++ passes the raw SelectTarget
-- result into DoCast — the Lua safe-harbor skips the nil cast).
-- EVENT_ARCANE_VOLLEY — DoCast(me, SPELL_ARCANE_VOLLEY 35059)
-- 2-arg (non-triggered); re-arm randtime(7s,12s).
-- EVENT_SLOW (heroic) — DoCast(me, SPELL_SLOW 35032) 2-arg
-- (non-triggered); re-arm randtime(15s,40s).
-- EVENT_BLINK — the IsNonMeleeSpellCast → InterruptNonMeleeSpells
-- leg rides the no-cast-state bridge (IsCasting is a stub returning
-- false, lua_objects.go:330) — documented-only; Talk(EMOTE_ARCANE_
-- EXPLOSION 4) + DoCastAOE(SPELL_BLINK 38194) 1-arg = non-triggered
-- self-cast → creature:CastSpell(creature, 38194); re-arm
-- randtime(35s,40s); schedules EVENT_ARCANE_EXPLOSION at 1s.
-- EVENT_ARCANE_EXPLOSION — DoCast(me, SPELL_ARCANE_EXPLOSION 38197)
-- 2-arg (non-triggered) + DoCast(me, SPELL_ARCANE_BUBBLE 9438, true)
-- 3-arg TRIGGERED; ONE-SHOT (C++ never re-arms it).
-- boss_talon_king_ikissAI::DamageTaken —
-- HealthBelowPctDamaged(20, damage) one-shot latch (event 9 fires
-- pre-damage — moroes precedent) → DoCast(me, SPELL_MANA_SHIELD
-- 38151) 2-arg (non-triggered); moroes-verified integer-floor
-- projection form.
-- boss_talon_king_ikissAI::JustDied — Talk(SAY_DEATH 3) (event 4;
-- sjonnir self-Talk precedent).
-- boss_talon_king_ikissAI::KilledUnit — TYPEID_PLAYER gate →
-- Talk(SAY_SLAY 2) (event 3; razuvious player-gated precedent).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- DOCUMENTED-ONLY (in this header; only entry 18473 registered):
-- spell_talon_king_ikiss_blink SpellScript (38194 → random
-- area-target teleport via SPELL_BLINK_TELEPORT 38203) — no
-- SpellScript bridge (instance_ulduar precedent).

local ENTRY_TALON_KING_IKISS = 18473 -- DB-side; no NPC_ constant in sethekk_halls.h

local SAY_INTRO = 0 -- boss_talon_king_ikiss.cpp:36
local SAY_AGGRO = 1 -- boss_talon_king_ikiss.cpp:37
local SAY_SLAY = 2 -- boss_talon_king_ikiss.cpp:38
local SAY_DEATH = 3 -- boss_talon_king_ikiss.cpp:39
local EMOTE_ARCANE_EXPLOSION = 4 -- boss_talon_king_ikiss.cpp:40

local SPELL_BLINK = 38194 -- boss_talon_king_ikiss.cpp:45
local SPELL_MANA_SHIELD = 38151 -- boss_talon_king_ikiss.cpp:47
local SPELL_ARCANE_BUBBLE = 9438 -- boss_talon_king_ikiss.cpp:48
local SPELL_SLOW = 35032 -- boss_talon_king_ikiss.cpp:49
local SPELL_POLYMORPH = 38245 -- boss_talon_king_ikiss.cpp:50
local SPELL_ARCANE_VOLLEY = 35059 -- boss_talon_king_ikiss.cpp:51
local SPELL_ARCANE_EXPLOSION = 38197 -- boss_talon_king_ikiss.cpp:52

local timers = {}
local ikissState = {}

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

-- C++ SelectTarget(Random, 0) (playerOnly defaults FALSE per
-- UnitAI.h:170 — pets/totems eligible) — threat-list pool via
-- GetAITargets (the bridged, threat-sorted list;
-- lua_creature_events.go:345-359); the Player filter was dropped
-- 2026-10-19.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_POLYMORPH (:123-131): heroic → random player target;
-- normal → SelectTarget(MaxThreat, 1), the SECOND threat entry (tank
-- excluded — netherspite/curator offset-1 precedent). Re-arm
-- randtime(15s,17500ms) — millisecond granularity.
local function polymorphTick(creature, guid)
    local target
    if creature:IsHeroic() then
        target = randomThreatTarget(creature)
    else
        -- C++ SelectTarget(MaxThreat, 1) (playerOnly defaults FALSE per
        -- UnitAI.h:170): the second ALIVE threat-list entry (offline refs
        -- skipped before the offset), pets/totems eligible — NOT the
        -- second-highest-threat player.
        local alive = {}
        for _, t in ipairs(creature:GetAITargets()) do
            if not t:IsDead() then
                alive[#alive + 1] = t
            end
        end
        target = alive[2]
    end
    if target then
        creature:CastSpell(target, SPELL_POLYMORPH)
    end
    schedule(guid, "polymorph", {15000, 17500}, function()
        polymorphTick(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_VOLLEY (:132-135): DoCast(me,
-- SPELL_ARCANE_VOLLEY) 2-arg (non-triggered); re-arm
-- randtime(7s,12s).
local function arcaneVolleyTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_VOLLEY)
    schedule(guid, "arcane_volley", {7000, 12000}, function()
        arcaneVolleyTick(creature, guid)
    end)
end

-- C++ EVENT_SLOW (:136-139, heroic): DoCast(me, SPELL_SLOW) 2-arg
-- (non-triggered); re-arm randtime(15s,40s).
local function slowTick(creature, guid)
    creature:CastSpell(creature, SPELL_SLOW)
    schedule(guid, "slow", {15000, 40000}, function()
        slowTick(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_EXPLOSION (:148-152): DoCast(me,
-- SPELL_ARCANE_EXPLOSION) 2-arg (non-triggered) + DoCast(me,
-- SPELL_ARCANE_BUBBLE, true) 3-arg TRIGGERED; one-shot.
local function arcaneExplosionTick(creature)
    creature:CastSpell(creature, SPELL_ARCANE_EXPLOSION)
    creature:CastSpell(creature, SPELL_ARCANE_BUBBLE, true)
end

-- C++ EVENT_BLINK (:140-147): Talk(EMOTE_ARCANE_EXPLOSION) +
-- DoCastAOE(SPELL_BLINK) 1-arg = non-triggered self-cast; re-arm
-- randtime(35s,40s); schedules EVENT_ARCANE_EXPLOSION at 1s. The
-- interrupt-non-melee leg rides the no-cast-state bridge
-- (documented-only).
local function blinkTick(creature, guid)
    creature:Talk(EMOTE_ARCANE_EXPLOSION)
    creature:CastSpell(creature, SPELL_BLINK)
    schedule(guid, "blink", {35000, 40000}, function()
        blinkTick(creature, guid)
    end)
    schedule(guid, "arcane_explosion", 1000, function()
        arcaneExplosionTick(creature)
    end)
end

-- C++ boss_talon_king_ikissAI::MoveInLineOfSight (:104-112): first
-- player within 100.0f → Intro latch + Talk(SAY_INTRO). Event 27
-- fires (event, creature, unit) at the aggro-scan engage branch; the
-- handler MUST NOT return true (that vetoes the engage —
-- lua_creature_events.go:509-524).
local function ikissMoveInLOS(event, creature, unit)
    local guid = creature:GetGUID()
    -- Lazy init: the C++ constructor sets Intro = false, so a spawn
    -- the engine never fired 23 for still gets its one intro.
    local state = ikissState[guid]
    if state == nil then
        state = {intro = false, manaShield = false}
        ikissState[guid] = state
    end
    if not state.intro
            and unit:GetObjectType() == "Player"
            and creature:GetDistance2d(unit) <= 100.0 then
        state.intro = true
        creature:Talk(SAY_INTRO)
    end
end

-- C++ boss_talon_king_ikissAI::JustEngagedWith (:114-122):
-- Talk(SAY_AGGRO) + schedules EVENT_ARCANE_VOLLEY 5s,
-- EVENT_POLYMORPH 8s, EVENT_BLINK 35s, heroic-only EVENT_SLOW
-- 15-30s rand.
local function ikissEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "arcane_volley", 5000, function()
        arcaneVolleyTick(creature, guid)
    end)
    schedule(guid, "polymorph", 8000, function()
        polymorphTick(creature, guid)
    end)
    schedule(guid, "blink", 35000, function()
        blinkTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "slow", {15000, 30000}, function()
            slowTick(creature, guid)
        end)
    end
end

local function ikissLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset (:96-101): Intro = false, ManaShield = false. The
-- constructor does the same, so lazy-init on 27/9 covers a spawn
-- the engine never fired 23 for.
local function ikissReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ikissState[guid] = {intro = false, manaShield = false}
end

-- C++ boss_talon_king_ikissAI::KilledUnit (:170-174):
-- if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY) — the
-- razuvious player-gated precedent (event 3).
local function ikissTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_talon_king_ikissAI::JustDied (:163-167): _JustDied()
-- (instance leg — documented-only) + Talk(SAY_DEATH) — the sjonnir
-- self-Talk precedent (event 4).
local function ikissDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
    ikissState[creature:GetGUID()] = nil
end

-- C++ boss_talon_king_ikissAI::DamageTaken (:155-161):
-- HealthBelowPctDamaged(20, damage) one-shot latch → DoCast(me,
-- SPELL_MANA_SHIELD) 2-arg (non-triggered). Event 9 fires
-- pre-damage (moroes precedent); moroes-verified integer-floor
-- projection form.
local function ikissDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = ikissState[guid]
    if state == nil then
        state = {intro = false, manaShield = false}
        ikissState[guid] = state
    end
    if state.manaShield then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = math.max(health - damage, 0)
    if postDamage < math.floor(maxHealth * 20 / 100) then
        creature:CastSpell(creature, SPELL_MANA_SHIELD)
        state.manaShield = true
    end
end

RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 1, ikissEnterCombat)
RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 2, ikissLeaveCombat)
RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 3, ikissTargetDied)
RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 4, ikissDied)
RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 9, ikissDamageTaken)
RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 23, ikissReset)
RegisterCreatureEvent(ENTRY_TALON_KING_IKISS, 27, ikissMoveInLOS)
