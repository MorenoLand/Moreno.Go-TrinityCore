-- Valithria Dreamwalker (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_valithria_dreamwalker.cpp
-- (1269 lines incl. license; 10 CreatureScripts
-- (boss_valithria_dreamwalker (ScriptedAI, DATA_VALITHRIA_DREAMWALKER = 10),
-- npc_green_dragon_combat_trigger (BossAI),
-- npc_the_lich_king_controller (ScriptedAI),
-- npc_risen_archmage / npc_blazing_skeleton / npc_suppresser /
-- npc_blistering_zombie / npc_gluttonous_abomination (ScriptedAI),
-- npc_dream_portal (CreatureAI), npc_dream_cloud (ScriptedAI)) +
-- 9 SpellScripts (spell_dreamwalker_mana_void,
-- spell_dreamwalker_decay_periodic_timer, spell_dreamwalker_summoner,
-- spell_dreamwalker_summon_suppresser,
-- spell_dreamwalker_summon_suppresser_effect,
-- spell_dreamwalker_summon_dream_portal,
-- spell_dreamwalker_summon_nightmare_portal,
-- spell_dreamwalker_nightmare_cloud,
-- spell_dreamwalker_twisted_nightmares) + 1 AchievementCriteriaScript
-- (achievement_portal_jockey); all registered from inside
-- AddSC_boss_valithria_dreamwalker(); loader decl 181 / call 376 per
-- northrend_script_loader.cpp — the ELEVENTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sister_svalna() (call 375) — verified from the
-- loader this run; the checkpoint sequence (sister_svalna ->
-- valithria_dreamwalker) is followed).
-- Entries: 36789 Valithria Dreamwalker (icecrown_citadel.h
-- NPC_VALITHRIA_DREAMWALKER, line 287; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_VALITHRIA_DREAMWALKER, line 291),
-- 16980 the lich king controller (icecrown_citadel.h
-- NPC_THE_LICH_KING_VALITHRIA, line 297; OnCreatureCreate binds case
-- NPC_THE_LICH_KING_VALITHRIA, line 294) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent); the
-- RegisterIcecrownCitadelCreatureAI ScriptName bindings are DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_valithria_dreamwalker" hits boss_valithria_dreamwalker.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction. No valithria lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat.
-- Ported arms (C++-exact for all modeled arms):
-- npc_the_lich_king_controller JustEngagedWith — Talk(SAY_LICH_KING_INTRO 0)
-- (event 1 — the auriaya engage-port precedent; the me->setActive(true)
-- leg has no bridge).
-- DOCUMENTED-ONLY (in this header; only entry 16980 registered):
-- boss_valithria_dreamwalker exposes zero bridgeable arms — NOT registered
-- (the bronjahm npc_corrupted_soul_fragment precedent), 8 Talk() calls
-- all on unbridged legs:
-- Talk(SAY_VALITHRIA_SUCCESS 7) rides the HealReceived 100%-heal leg with
-- the _done flag (no HealReceived bridge; the DoCastSelf/DoCastAOE /
-- instance->SendEncounterUnit / RemoveAuras legs have no bridges);
-- Talk(SAY_VALITHRIA_75_PERCENT 2) rides the HealReceived 75%-threshold leg
-- with the _over75PercentTalkDone flag (no HealReceived bridge);
-- PORTED (2026-10-13): Talk(SAY_VALITHRIA_25_PERCENT 3) — the
-- DamageTaken HealthBelowPctDamaged(25, damage) leg with the
-- _under25PercentTalkDone latch (event 9; ironaya floored-integer
-- convention — algalon/moroes precedent). The death-negation leg
-- (damage = 0, _justDied flag) has no bridge — a lone damage=0 would
-- strand the encounter (algalon precedent);
-- Talk(SAY_VALITHRIA_DEATH 4) rides the DamageTaken death-negated leg
-- (damage = 0, _justDied flag — she never actually dies; the trigger
-- DoAction(ACTION_DEATH) cross-AI leg has no bridge);
-- Talk(SAY_VALITHRIA_ENTER_COMBAT 0) rides UpdateAI EVENT_INTRO_TALK /
-- Talk(SAY_VALITHRIA_BERSERK 6) rides UpdateAI EVENT_BERSERK (heroic-only)
-- — both schedules fire from DoAction ACTION_ENTER_COMBAT (:305-313),
-- which is itself DoAction'ed from the bridge-blocked engagement chain
-- (:505-512: instance GUID fetch + DoZoneInCombat + SetBossState) — no
-- DoAction bridge (the timer bridge is CreateLuaEvent, live — the leg
-- origin is the blocker, not the timer);
-- Talk(SAY_VALITHRIA_DREAM_PORTAL 1) rides UpdateAI EVENT_DREAM_PORTAL
-- (non-heroic scheduler leg; the SUMMON_PORTAL DoCast legs have no-cast
-- bridge).
-- SAY_VALITHRIA_PLAYER_DEATH 5 is declared in the text enum but never
-- referenced by a Talk() call — dead text evidence (no KilledUnit arm
-- exists in the file).
-- npc_the_lich_king_controller Reset scheduler legs (5 summon-timer
-- schedules), JustSummoned phase-mask / DoZoneInCombat legs, UpdateAI
-- summoner machine and the 5 DoCastSelf SPELL_TIMER_* legs
-- (70915/70912/70914/70916/70913) — the whole summon effect rides the
-- spell_dreamwalker_summoner SpellScript + instance-GUID force-cast
-- machine (no-SpellScript bridge; the summon-machine rule);
-- JustReachedHome me->setActive(false) (no bridge).
-- npc_green_dragon_combat_trigger (38752) — no Talk; the DoAction
-- ACTION_ENTER_COMBAT / ACTION_SETUP_ARCHMAGES / ACTION_DEATH machine
-- (no-DoAction bridge) — NOT registered.
-- PORTED (2026-10-13 retraction re-audit): npc_risen_archmage
-- (37868) — FROSTBOLT_VOLLEY 70759 self-cast {5s,15s}/{8s,15s};
-- MANA_VOID 71179 random-player {20s,25s}/{20s,25s} (the POWER_MANA
-- type filter has no bridge — moroes approximation, dist>15.0f kept);
-- COLUMN_OF_FROST 70704 random-player {10s,20s}/{15s,25s},
-- dist>10.0f (:646-648/:705-721); npc_blazing_skeleton (36791) —
-- FIREBALL 70754 victim-cast {2s,4s}/{2s,4s}, !IsWithinMeleeRange
-- gated (5-yd check, the venoxis precedent); LEY_WASTE 69325
-- self-cast {15s,20s}/{15s,20s} (:745-749/:760-773).
-- npc_suppresser (37863) — zero Talk lines; SUPPRESSION 70588 is
-- MovementInform CHASE_MOTION_TYPE-driven (no bridge) — NOT
-- registered.
-- PORTED (2026-10-13): npc_blistering_zombie (37934) JustDied —
-- DoCastSelf(SPELL_ACID_BURST 70744, triggered); npc_gluttonous_abomination
-- (37886) JustDied — DoCastSelf(SPELL_ROT_WORM_SPAWNER 70675, triggered)
-- unless killed by Valithria (event 4; the_black_morass killer-gate precedent).
-- PORTED (2026-10-13 retraction re-audit): npc_gluttonous_abomination —
-- GUT_SPRAY 70633 self-cast {10s,13s} init/re-arm (:871-872/:899).
-- npc_dream_portal (37945) / npc_dream_cloud (entry DB-side, no header
-- constant) — zero Talk lines; portal JustSummoned delayed-cast event
-- legs (ValithriaDelayedCastEvent / ValithriaAuraRemoveEvent — no
-- event-machine bridge), MISSED_PORTALS GetData leg (no bridge) — NOT
-- registered.
-- The 9 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent); achievement_portal_jockey joins the
-- no-achievement-criteria bridge queue (the lana'thel achievement
-- precedent).

local ENTRY_LICH_KING_VALITHRIA = 16980
local ENTRY_VALITHRIA_DREAMWALKER = 36789
local ENTRY_BLISTERING_ZOMBIE = 37934
local ENTRY_GLUTTONOUS_ABOMINATION = 37886

local SAY_LICH_KING_INTRO = 0
local SAY_VALITHRIA_25_PERCENT = 3
local SAY_VALITHRIA_DEATH = 4

local SPELL_ACID_BURST = 70744
local SPELL_ROT_WORM_SPAWNER = 70675

local ENTRY_RISEN_ARCHMAGE = 37868
local ENTRY_BLAZING_SKELETON = 36791

local SPELL_GUT_SPRAY = 70633
local SPELL_FROSTBOLT_VOLLEY = 70759
local SPELL_MANA_VOID = 71179
local SPELL_COLUMN_OF_FROST = 70704
local SPELL_FIREBALL = 70754
local SPELL_LEY_WASTE = 69325

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

-- C++ random alive player with a minimum distance, on the same
-- map (the falric / keleseth / thorim GetPlayersInWorld precedent).
local function randomPlayer(creature, minDist)
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:IsAlive() and creature:GetMap() == p:GetMap()
                and creature:GetDistance(p) > minDist then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ npc_gluttonous_abomination::JustEngagedWith (Reset seeds the
-- same timers; the maiden engage-hook convention): EVENT_GUT_SPRAY —
-- DoCastSelf(SPELL_GUT_SPRAY) non-triggered, {10s,13s}/{10s,13s}
-- (:871-872/:899).
local function onGutSpray(creature, guid)
    creature:CastSpell(creature, SPELL_GUT_SPRAY)
    schedule(guid, "gut_spray", {10000, 13000}, function() onGutSpray(creature, guid) end)
end

local function gluttonousAbominationJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "gut_spray", {10000, 13000}, function() onGutSpray(creature, guid) end)
end

local function gluttonousAbominationLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_risen_archmage::Reset — EVENT_FROSTBOLT_VOLLEY {5s,15s},
-- EVENT_MANA_VOID {20s,25s}, EVENT_COLUMN_OF_FROST {10s,20s}
-- (:646-648).
local function onFrostboltVolley(creature, guid)
    creature:CastSpell(creature, SPELL_FROSTBOLT_VOLLEY)
    schedule(guid, "frostbolt_volley", {8000, 15000}, function() onFrostboltVolley(creature, guid) end)
end

local function onManaVoid(creature, guid)
    local target = randomPlayer(creature, 15.0)
    if target then
        creature:CastSpell(target, SPELL_MANA_VOID)
    end
    schedule(guid, "mana_void", {20000, 25000}, function() onManaVoid(creature, guid) end)
end

local function onColumnOfFrost(creature, guid)
    local target = randomPlayer(creature, 10.0)
    if target then
        creature:CastSpell(target, SPELL_COLUMN_OF_FROST)
    end
    schedule(guid, "column_of_frost", {15000, 25000}, function() onColumnOfFrost(creature, guid) end)
end

local function risenArchmageJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "frostbolt_volley", {5000, 15000}, function() onFrostboltVolley(creature, guid) end)
    schedule(guid, "mana_void", {20000, 25000}, function() onManaVoid(creature, guid) end)
    schedule(guid, "column_of_frost", {10000, 20000}, function() onColumnOfFrost(creature, guid) end)
end

local function risenArchmageLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_blazing_skeleton::Reset — EVENT_FIREBALL {2s,4s},
-- EVENT_LEY_WASTE {15s,20s} (:745-749); fireball gated on
-- !IsWithinMeleeRange — 5-yd check (the venoxis precedent).
local function onFireball(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:GetDistance(victim) > 5 then
        creature:CastSpell(nil, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", {2000, 4000}, function() onFireball(creature, guid) end)
end

local function onLeyWaste(creature, guid)
    creature:CastSpell(creature, SPELL_LEY_WASTE)
    schedule(guid, "ley_waste", {15000, 20000}, function() onLeyWaste(creature, guid) end)
end

local function blazingSkeletonJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fireball", {2000, 4000}, function() onFireball(creature, guid) end)
    schedule(guid, "ley_waste", {15000, 20000}, function() onLeyWaste(creature, guid) end)
end

local function blazingSkeletonLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_the_lich_king_controller::JustEngagedWith:
-- Talk(SAY_LICH_KING_INTRO) — the auriaya engage-port precedent.
local function lichKingJustEngagedWith(event, creature, target)
    creature:Talk(SAY_LICH_KING_INTRO)
end

local valithriaState = {}

-- C++ boss_valithria_dreamwalker::DamageTaken — HealthBelowPctDamaged(
-- 25, damage) = (health - damage) < floor(max*25/100) (Unit.h:873;
-- ironaya floored-integer convention) + the _under25PercentTalkDone
-- latch — Talk(SAY_VALITHRIA_25_PERCENT) (:348-356); the lethal leg
-- (damage >= health) negates damage to 0 + the _justDied latch —
-- Talk(SAY_VALITHRIA_DEATH) (:357-366). In C++ she never dies; the
-- encounter completion rides the cross-AI ACTION_DEATH leg (no
-- bridge), but the no-death + Talk behavior is exactly bridgeable
-- (return false, 0 — julianne/curator damage-negation precedent).
local function valithriaDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local st = valithriaState[guid] or {}
    valithriaState[guid] = st
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 25 / 100) then
        if not st.talk25Done then
            st.talk25Done = true
            creature:Talk(SAY_VALITHRIA_25_PERCENT)
        end
        if damage >= creature:GetHealth() then
            if not st.justDied then
                st.justDied = true
                creature:Talk(SAY_VALITHRIA_DEATH)
            end
            return false, 0
        end
    end
    return false, damage
end

local function valithriaReset(event, creature)
    valithriaState[creature:GetGUID()] = nil
end

-- C++ npc_blistering_zombie::JustDied — unconditional
-- DoCastSelf(SPELL_ACID_BURST, true) (triggered, :851-853).
local function blisteringZombieJustDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_ACID_BURST, true)
end

-- C++ npc_gluttonous_abomination::JustDied —
-- DoCastSelf(SPELL_ROT_WORM_SPAWNER, true) (triggered) unless killed
-- by Valithria herself (:873-878).
local function gluttonousAbominationJustDied(event, creature, killer)
    if killer and killer:GetEntry() == ENTRY_VALITHRIA_DREAMWALKER then
        return
    end
    creature:CastSpell(creature, SPELL_ROT_WORM_SPAWNER, true)
end

RegisterCreatureEvent(ENTRY_LICH_KING_VALITHRIA, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 9, valithriaDamageTaken)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 23, valithriaReset)
RegisterCreatureEvent(ENTRY_BLISTERING_ZOMBIE, 4, blisteringZombieJustDied)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 4, gluttonousAbominationJustDied)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 1, gluttonousAbominationJustEngagedWith)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 2, gluttonousAbominationLeaveCombat)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 23, gluttonousAbominationLeaveCombat)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 1, risenArchmageJustEngagedWith)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 2, risenArchmageLeaveCombat)
RegisterCreatureEvent(ENTRY_RISEN_ARCHMAGE, 23, risenArchmageLeaveCombat)
RegisterCreatureEvent(ENTRY_BLAZING_SKELETON, 1, blazingSkeletonJustEngagedWith)
RegisterCreatureEvent(ENTRY_BLAZING_SKELETON, 2, blazingSkeletonLeaveCombat)
RegisterCreatureEvent(ENTRY_BLAZING_SKELETON, 23, blazingSkeletonLeaveCombat)
