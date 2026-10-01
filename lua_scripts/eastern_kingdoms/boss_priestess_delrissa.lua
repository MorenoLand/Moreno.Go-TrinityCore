-- Priestess Delrissa (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp
-- (boss_priestess_delrissa — boss class only; the file's eight lackey
-- CreatureScript classes (boss_kagani_nightstrike, boss_ellris_
-- duskhallow, boss_eramas_brightblaze, boss_yazzai, boss_warlord_
-- salaris, boss_garaxxas, boss_apoko, boss_zelfan) plus the commented-
-- out npc_high_explosive_sheep are queued for later runs — lackey
-- machine parts (lackey GUID lists, threat adds, instance calls) have
-- no bridges here). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_vexallus done: boss
-- class ported, entry 24744 registered; AddSC_boss_priestess_delrissa
-- next in the Magister's Terrace block; instance_magisters_terrace.cpp
-- stays blocked on the instance-script model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h names
-- BOSS_PRIESTESS_DELRISSA = 24560 in the MTCreatureIds enum. The AI
-- is ScriptedAI (not BossAI) driven by DATA_PRIESTESS_DELRISSA = 2
-- via GetMagistersTerraceAI; the ported arms carry no instance state.
-- The creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_felblood_kaelthas precedent). Script name is the
-- stringified class name (CreatureScript("boss_priestess_delrissa");
-- RegisterCreatureAIWithFactory convention, felblood precedent).
-- Verifiable numbers (file's own enums/header): SAY_AGGRO = 0 /
-- SAY_DEATH = 10; LackeyDeath ids 1..4; PlayerDeath ids 5..9 /
-- SPELL_DISPEL_MAGIC = 27609 / SPELL_FLASH_HEAL = 17843 /
-- SPELL_SW_PAIN_NORMAL = 14032 / SPELL_SW_PAIN_HEROIC = 15654 /
-- SPELL_SHIELD = 44291 / SPELL_RENEW_NORMAL = 44174 /
-- SPELL_RENEW_HEROIC = 46192 / MAX_ACTIVE_LACKEY = 4 /
-- DATA_PRIESTESS_DELRISSA = 2 (MTDataTypes) /
-- DATA_DELRISSA_DEATH_COUNT = 5.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied (C++ Eluna::KilledUnit — lua_creature_events.go fires it
-- with (event, creature, victim) from Unit::Kill), 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith + UpdateAI heal/dispel/shield timers
-- + KilledUnit + JustDied):
-- Combat entry: Talk(SAY_AGGRO) on OnEnterCombat(1) (maiden-of-virtue
-- convention).
-- Heal machine: flash-heal self-cast 17843 15000ms -> 15000ms loop.
-- In C++ the heal targets the alive lackey with the least health and
-- falls back to self; the lackey-GUID list (ObjectAccessor) has no
-- bridge, so the port models the self-target leg only (self-cast via
-- creature:CastSpell(creature, spell), selin DoCastAOE convention).
-- Renew machine: renew self-cast 44174 10000ms -> 5000ms loop (the
-- C++ leg that targets a random alive lackey is the same GUID-list
-- bridge; the heroic 46192 leg is dropped — no difficulty bridge).
-- Shield machine: shield self-cast 44291 2000ms -> 7500ms loop (the
-- C++ random-lackey leg and the HasAura(44291) check share the same
-- missing bridges).
-- Dispel machine: dispel-magic self-cast 27609 7500ms -> 12000ms loop.
-- The C++ target chooser (random player via SelectTarget / self /
-- random lackey via GUID list) has no bridges on either leg; the port
-- models the self-cast leg only.
-- KilledUnit: via OnTargetDied(3); the C++ victim->GetTypeId() ==
-- TYPEID_PLAYER gate is bridged by victim:GetObjectType() == "Player"
-- (selin precedent): Talk(PlayerDeath[PlayersKilled]) with a per-guid
-- counter incremented after each talk and capped at 4, i.e. ids
-- 5,6,7,8,9,9,9... (C++-exact: Talk then ++PlayersKilled only when
-- PlayersKilled < 4). The counter resets on OnEnterCombat(1) (C++
-- Initialize runs in the constructor and Reset — per-engagement reset
-- is the gyth/vaelastrasz convention).
-- JustDied: Talk(SAY_DEATH) on OnDied(4) (najentus/halycon precedent).
-- Timers start on OnEnterCombat(1), cancelled on OnLeaveCombat(2)/
-- OnDied(4)/OnReset(23) (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so the timers fire unconditionally and the in-loop
-- casting gates are dropped (maiden precedent). The C++ scheduler is
-- driven from JustEngagedWith and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden
-- convention).
-- Reset() _Reset() / JustDied() _JustDied() are internal ScriptedAI
-- machinery covered by the cancel/re-arm on combat events (halycon
-- precedent); Reset's InitializeLackeys() summon/resummon machine has
-- no summon bridge (below).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset()'s InitializeLackeys(): the 4-of-8 random lackey selection
--   and SummonCreature calls (TEMPSUMMON_CORPSE_DESPAWN) at the four
--   LackeyLocations plus the resummon-on-missing leg — no summon
--   bridge on the Lua surface (razorgore precedent).
-- - JustReachedHome: instance->SetBossState(DATA_PRIESTESS_DELRISSA,
--   FAIL) — no instance bridge (standing blocker).
-- - JustEngagedWith's lackey AddThreat(who, 0.0f, pAdd) ring via the
--   lackey GUID list (ObjectAccessor) — no bridge; the SetBossState
--   IN_PROGRESS arm — instance bridge.
-- - JustDied's DATA_DELRISSA_DEATH_COUNT check and the lootable-flag
--   manipulation — instance bridge.
-- - SW_PAIN machine: DoCast(SelectTarget(Random, 0, 100, true),
--   SPELL_SW_PAIN_NORMAL 14032) — no SelectTarget bridge (the_beast /
--   doomwalker precedent). The heroic 15654 leg is dropped with it.
-- - ResetTimer's home-position Z+10 evade check (EnterEvadeMode) — no
--   evade/home bridge on the Lua surface (the_beast evade precedent).
-- - The eight lackey CreatureScript classes + commented-out
--   npc_high_explosive_sheep: each lackey inherits
--   boss_priestess_lackey_commonAI (AcquireGUIDs / threat-reset timer
--   / JustDied death-count talk to Delrissa / KilledUnit forward /
--   sub-25% healing-potion latch / Reset respawn of Delrissa) and adds
--   its own spell machine — all gate on the instance + lackey-GUID +
--   SelectTarget bridges. Their entries come from the file's own
--   m_auiAddEntries array: 24557 Kagani Nightstrike, 24558 Elris
--   Duskhallow, 24554 Eramas Brightblaze, 24561 Yazzaj, 24559 Warlord
--   Salaris, 24555 Garaxxas, 24553 Apoko, 24556 Zelfan (plus pet
--   NPC_SLIVER = 24552, summoned — no summon bridge). No .lua files
--   written for them here (stub-free precedent, grubbis / boss_lord_
--   valthalak queue precedent); queued for later runs once bridges
--   exist or as future genuinely-bridgeable slices.
-- - Kagani (rogue): gouge 12540 5500ms loop, kick 27613 7000ms loop,
--   eviscerate 27611 6000ms->4000ms loop are DoCastVictim (bridgeable
--   later); vanish 44290 + SelectTarget add-threat switch + backstab
--   15657 / kidney-shot 27615 reappear — SelectTarget bridge.
-- - Ellris (warlock): immolate 44267 / shadow-bolt 12471 victim loops
--   (bridgeable later); summon-imp 44163 JustEngagedWith (no summon
--   bridge); seed-of-corruption 44141 / curse-of-agony 14875 / fear
--   38595 — SelectTarget bridge.
-- - Eramas (monk): knockdown 11428 / snap-kick 46182 victim loops
--   (bridgeable later).
-- - Yazzai (mage): ice-lance 46194 / cone-of-cold 38384 / frostbolt
--   15043 victim loops (bridgeable later); sub-35% ice-block 27619
--   self-latch; polymorph 13323 / blizzard 44178 — SelectTarget
--   bridge; blink 14514 — combat-manager melee-range bridge.
-- - Salaris (warrior): disarm 27581 / hamstring 27584 / mortal-strike
--   44268 / piercing-howl 23600 / frightening-shout 19134 victim
--   loops + battle-shout 27578 JustEngagedWith self-cast (bridgeable
--   later); intercept 27577 — combat-manager melee-range +
--   SelectTarget bridges.
-- - Garaxxas (hunter): aimed-shot 44271 / shoot 15620 / concussive-
--   shot 27634 / multi-shot 31942 victim loops (bridgeable later);
--   wing-clip 44286 / freezing-trap 44136 — victim-distance +
--   gameobject bridges; sliver pet summon (no summon bridge).
-- - Apoko (shaman): totem roulette RAND(27621/44257/15786) self-cast,
--   war-stomp 46026 self-cast, frost-shock 21401 victim, lesser-
--   healing-wave 44256 self (bridgeable later); purge 27626 —
--   SelectTarget bridge.
-- - Zelfan (engineer): goblin-dragon-gun 44272 / rocket-launch 44137 /
--   fel-iron-bomb 46024 victim loops + high-explosive-sheep 44276
--   self-cast (bridgeable later); recombobulate 44274 — lackey GUID
--   list + IsPolymorphed bridges.
local SAY_AGGRO = 0
local SAY_DEATH = 10

local SPELL_FLASH_HEAL = 17843
local SPELL_RENEW_NORMAL = 44174
local SPELL_SHIELD = 44291
local SPELL_DISPEL_MAGIC = 27609

local PlayerDeath = { 5, 6, 7, 8, 9 }
local MAX_PLAYER_DEATH_TALK = 4

local ENTRY_PRIESTESS_DELRISSA = 24560

local timers = {}
local playersKilled = {}

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

local function onHeal(creature, guid)
    creature:CastSpell(creature, SPELL_FLASH_HEAL)
    schedule(guid, "heal", 15000, function() onHeal(creature, guid) end)
end

local function onRenew(creature, guid)
    creature:CastSpell(creature, SPELL_RENEW_NORMAL)
    schedule(guid, "renew", 5000, function() onRenew(creature, guid) end)
end

local function onShield(creature, guid)
    creature:CastSpell(creature, SPELL_SHIELD)
    schedule(guid, "shield", 7500, function() onShield(creature, guid) end)
end

local function onDispel(creature, guid)
    creature:CastSpell(creature, SPELL_DISPEL_MAGIC)
    schedule(guid, "dispel", 12000, function() onDispel(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    playersKilled[guid] = 0
    creature:Talk(SAY_AGGRO)
    schedule(guid, "heal", 15000, function() onHeal(creature, guid) end)
    schedule(guid, "renew", 10000, function() onRenew(creature, guid) end)
    schedule(guid, "shield", 2000, function() onShield(creature, guid) end)
    schedule(guid, "dispel", 7500, function() onDispel(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(_, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        local guid = creature:GetGUID()
        local n = playersKilled[guid] or 0
        if n > MAX_PLAYER_DEATH_TALK then
            n = MAX_PLAYER_DEATH_TALK
        end
        creature:Talk(PlayerDeath[n + 1])
        if n < MAX_PLAYER_DEATH_TALK then
            playersKilled[guid] = n + 1
        end
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 4, onDied)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 23, onCombatEnd)
