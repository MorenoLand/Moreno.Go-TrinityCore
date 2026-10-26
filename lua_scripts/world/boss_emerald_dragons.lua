-- Emerald Dragons (world bosses) — Lua port of
-- src/server/scripts/World/boss_emerald_dragons.cpp
-- (4 CreatureScripts: boss_ysondre / boss_lethon / boss_emeriss /
-- boss_taerar (WorldBossAI over the shared emerald_dragonAI base) +
-- npc_dream_fog (ScriptedAI, NPC_DREAM_FOG = 15224, zero Talk) +
-- npc_spirit_shade (PassiveAI, NPC_SPIRIT_SHADE = 15261, zero Talk);
-- 2 SpellScriptLoaders (spell_dream_fog_sleep, spell_mark_of_nature);
-- all registered from inside AddSC_emerald_dragons() (line 808);
-- loader decl 23 / call 44 per world_script_loader.cpp — the group
-- immediately after AddSC_areatrigger_scripts() (call 43) in
-- AddWorldScripts() — verified from the loader this run; the
-- checkpoint sequence (areatrigger_scripts -> emerald_dragons) is
-- followed).
-- Entries: 14887 Ysondre (Feralas) / 14888 Lethon (Duskwood) /
-- 14889 Emeriss (Hinterlands) / 14890 Taerar (Ashenvale) — all from
-- the file's own EmeraldDragonNPC enum; NPC_DREAM_FOG = 15224 from the
-- same enum; the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_emerald_dragons" hits boss_emerald_dragons.cpp only (+ the
-- loader decl/call lines). No emerald-dragons lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent.
-- Ported arms (C++-exact for all modeled arms) — the four
-- JustEngagedWith arms (event 1 — the auriaya precedent; the
-- WorldBossAI::JustEngagedWith legs have no bridges):
-- boss_ysondre JustEngagedWith — Talk(SAY_YSONDRE_AGGRO 0) +
-- EVENT_LIGHTNING_WAVE (DoCastVictim 24819, 12s init,
-- randtime(10s,20s) repeat);
-- boss_lethon JustEngagedWith — Talk(SAY_LETHON_AGGRO 0) +
-- EVENT_SHADOW_BOLT_WHIRL (me->CastSpell(nullptr, 24834, false),
-- non-triggered, 10s init, randtime(15s,30s) repeat — lowered to
-- self-cast);
-- boss_emeriss JustEngagedWith — Talk(SAY_EMERISS_AGGRO 0) +
-- EVENT_VOLATILE_INFECTION (DoCastVictim 24928, 12s init, 120s
-- repeat);
-- boss_taerar JustEngagedWith — Talk(SAY_TAERAR_AGGRO 0) +
-- EVENT_ARCANE_BLAST (DoCast 24857 1-arg — AISpellInfo resolves to
-- AITARGET_VICTIM for the ranged nuke, so victim-cast; 12s init,
-- randtime(7s,12s) repeat) + EVENT_BELLOWING_ROAR (DoCast 22686 1-arg
-- — AISpellInfo stays AITARGET_SELF for the range-0 PBAOE, so
-- self-cast; 30s init, randtime(20s,30s) repeat).
-- (The Oct-12 "no-timer bridge" claim was wrong — CreateLuaEvent is
-- a real bridge, retracted this run.) All timers cancelled on
-- 2/4/23 (gargolmar precedent).
-- PORTED (2026-10-19): boss_taerarAI::Reset's RemoveAurasDueToSpell(
-- SPELL_SHADE 24313) — taerarReset; C++-exact (apply leg unported, so a
-- no-op today).
-- DOCUMENTED-ONLY (in this header; only the four dragon entries
-- registered):
-- emerald_dragonAI Reset — PORTED 2026-10-24: RemoveFlag NOT_SELECTABLE /
-- NON_ATTACKABLE (already ported 2026-10-20) + DoCast(me, SPELL_MARK_OF_
-- NATURE_AURA 25041, triggered) self-cast in dragonReset; the aura is
-- mechanically inert in Go (its 10s tick triggers 25042 via the
-- SpellScript-only spell_mark_of_nature), like the ported KilledUnit
-- 25040 cast. SetReactState(REACT_AGGRESSIVE) stays out: nothing in the
-- ported model ever sets the dragons non-aggressive (see dragonReset).
-- The base EVENT_TAIL_SWEEP / EVENT_NOXIOUS_BREATH / EVENT_SEEPING_FOG
-- machine is PORTED 2026-10-24 (tailSweepTick / noxiousBreathTick /
-- seepingFogTick, seeded in all four EnterCombat handlers): the old
-- claim that the events ride the SPELL_SUMMON_PLAYER 24776 leg was
-- wrong — 24776 is a separate UpdateAI leg (:158-159), not part of
-- ExecuteEvent. That leg is PORTED 2026-10-24 too (summonPlayerTick,
-- 1s combat pump): MaxThreat player beyond 50yd via the threat-ordered
-- GetAITargets list + the reach-adjusted distance bar (UnitAI.cpp
-- DefaultTargetSelector: negative dist excludes in-range targets).
-- The three events are plain self-casts with their own re-arms.
-- emerald_dragonAI KilledUnit — PORTED (this run): player-gated
-- who->CastSpell(who, SPELL_MARK_OF_NATURE 25040, triggered), lowered to
-- creature:CastSpell(victim, 25040, true) (maiden_of_virtue / moroes
-- triggered-cast convention); the aura lands on the same target.
-- Mechanically inert in Go (no spell_mark_of_nature effect), like the
-- ported DamageTaken visual casts in this file — the old
-- "victim-self-cast / no-caster-override" non-port rationale is
-- retracted.
-- boss_emeriss KilledUnit — PORTED (this run): DoCast(who, SPELL_PUTRID_
-- MUSHROOM 24904, triggered) + the base mark leg, same convention.
-- boss_ysondre / boss_lethon / boss_emeriss DamageTaken — PORTED
-- (this run): OnDamageTaken event 9 is bridged (midnight never-die
-- clamp precedent), so the stale "no-DamageTaken bridge" claims are
-- retracted — Talk + cast + ++_stage, pre-damage health (event 9 fires
-- pre-damage, moroes precedent), C++-exact floor comparisons
-- (!HealthAbovePct(p) = health <= floor(max*p/100)). The three cast
-- spells (24795/24811/24910) have no Go effect implementation —
-- visual-only casts like midnight's SPELL_SUMMON_ATTUMEN.
-- boss_lethon SpellHitTarget — PORTED (lethonSpellHitTarget, ev15): the
-- stale "no-SpellHitTarget bridge" claim was retracted 2026-10-21 (ev15
-- CREATURE_EVENT_ON_SPELL_HIT_TARGET bridged, runtime.go:228; fired
-- boss_ai.go:2280-2287) and the stale "no summon bridge" claim retracted
-- 2026-10-23 (creature:SummonCreature live, lua_creature_events.go:728):
-- DRAW_SPIRIT hit-target -> SummonCreature(NPC_SPIRIT_SHADE 15261) at the
-- target's position, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT Eluna type 3,
-- 50s, with the C++ TYPEID_PLAYER gate.
-- boss_taerar DamageTaken — Talk(SAY_TAERAR_SUMMON_SHADES 1) rides the
-- 25%-per-stage banish machine (3x triggered victim-cast shade summons
-- 24841/24842/24843 + DoCast SPELL_SHADE 24313 + NOT_SELECTABLE /
-- NON_ATTACKABLE flags + REACT_PASSIVE + 60s _banishedTimer) — event 9
-- IS bridged, and SetFlag/RemoveFlag (2026-10-23) + SetReactState
-- (lana_thel precedent) + AttackStop (2026-10-22) are now bridged too,
-- so the arm stays out only on the banish machine's unported shade
-- summons (24841/24842/24843 — no bearer, mandokir/nefarian precedent;
-- the _shades-- leg gates the banish exit) and the
-- no-InterruptNonMeleeSpells bridge (verified STAND 2026-10-23); the
-- flag/react/AttackStop legs ride the machine. The stale
-- "no-DamageTaken bridge" claim is retracted, and the stale
-- "no-SummonedCreatureDies bridge" claim is retracted too (event-21
-- dispatch bridged 2026-10-24) — the blocker is the missing summons,
-- not the dispatch.
-- The four dragon ExecuteEvent arms ported above (EVENT_LIGHTNING_WAVE
-- / EVENT_SHADOW_BOLT_WHIRL / EVENT_VOLATILE_INFECTION /
-- EVENT_ARCANE_BLAST / EVENT_BELLOWING_ROAR).
-- npc_dream_fog — zero Talk; NOT registered: the _roamTimer 15-30s
-- chase / 2.5s MoveRandom + MoveChase + SetWalk + walk-speed machine
-- — re-keyed 2026-10-15 (timer doctrine): no-movement bridge is the
-- true blocker (the timer leg bridges); with no registered Lua file
-- the class stays out. The stale "no-timer" claim is corrected.
-- npc_spirit_shade — zero Talk; NOT registered: the IsSummonedBy
-- MoveFollow + MovementInform dark-offering cast + 1s
-- DespawnOrUnsummon machine — no-movement / no-summon-death /
-- no-despawn bridges are the true blockers (the timer and cast legs
-- bridge); with no registered Lua file the class stays out. The
-- stale "no-timer / no-cast" claim is corrected.
-- spell_dream_fog_sleep + spell_mark_of_nature (SpellScriptLoaders) —
-- no-SpellScript bridge — NOT registered.
-- All Talk() calls in the file accounted for (4 ported above +
-- SAY_YSONDRE_SUMMON_DRUIDS 1 + SAY_LETHON_DRAW_SPIRIT 1 +
-- SAY_EMERISS_CAST_CORRUPTION 1 + SAY_TAERAR_SUMMON_SHADES 1
-- documented above).

local ENTRY_YSONDRE = 14887
local ENTRY_LETHON  = 14888
local ENTRY_EMERISS = 14889
local ENTRY_TAERAR  = 14890

local ENTRY_SPIRIT_SHADE = 15261 -- NPC_SPIRIT_SHADE, boss_emerald_dragons.cpp:388

-- UnitDefines.h:125/149; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59)
-- is bridged (engine/world/lua_creature_events.go:690/725).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local SAY_AGGRO = 0 -- SAY_YSONDRE_AGGRO / SAY_LETHON_AGGRO /
                    -- SAY_EMERISS_AGGRO / SAY_TAERAR_AGGRO (all 0)

local SPELL_LIGHTNING_WAVE = 24819 -- boss_emerald_dragons.cpp:248 (Ysondre)
local SPELL_SHADOW_BOLT_WHIRL = 24834 -- boss_emerald_dragons.cpp:337 (Lethon)
local SPELL_VOLATILE_INFECTION = 24928 -- boss_emerald_dragons.cpp:475 (Emeriss)
local SPELL_ARCANE_BLAST = 24857 -- boss_emerald_dragons.cpp:568 (Taerar)
local SPELL_BELLOWING_ROAR = 22686 -- boss_emerald_dragons.cpp:563 (Taerar)
local SPELL_SHADE = 24313 -- boss_emerald_dragons.cpp:563 (Taerar self-cast
    -- in the 25%-stage banish machine; cleared by boss_taerarAI::Reset)
local SPELL_SUMMON_DRUID_SPIRITS = 24795 -- boss_emerald_dragons.cpp:249 (Ysondre DamageTaken)
local SPELL_DRAW_SPIRIT = 24811 -- boss_emerald_dragons.cpp:336 (Lethon DamageTaken)
local SPELL_CORRUPTION_OF_EARTH = 24910 -- boss_emerald_dragons.cpp:474 (Emeriss DamageTaken)
local SPELL_MARK_OF_NATURE = 25040 -- boss_emerald_dragons.cpp:53,112 (all-dragon KilledUnit)
local SPELL_TAIL_SWEEP = 15847 -- boss_emerald_dragons.cpp:46 (base ExecuteEvent)
local SPELL_NOXIOUS_BREATH = 24818 -- boss_emerald_dragons.cpp:52 (base ExecuteEvent)
local SPELL_SEEPING_FOG_LEFT = 24813 -- boss_emerald_dragons.cpp:50 (base ExecuteEvent)
local SPELL_SEEPING_FOG_RIGHT = 24814 -- boss_emerald_dragons.cpp:51 (base ExecuteEvent)
local SPELL_MARK_OF_NATURE_AURA = 25041 -- boss_emerald_dragons.cpp:54 (base Reset)
local SPELL_SUMMON_PLAYER = 24776 -- boss_emerald_dragons.cpp:47 (base UpdateAI)
local SPELL_PUTRID_MUSHROOM = 24904 -- boss_emerald_dragons.cpp:473,505 (Emeriss KilledUnit)

local SAY_SUMMON = 1 -- SAY_YSONDRE_SUMMON_DRUIDS / SAY_LETHON_DRAW_SPIRIT /
                     -- SAY_EMERISS_CAST_CORRUPTION (all 1)

-- per-GUID C++ _stage (Initialize/Reset sets 1; stages at 75/50/25%).
local stages = {}

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

-- C++ boss_ysondre::ExecuteEvent (:299-301): DoCastVictim(SPELL_
-- LIGHTNING_WAVE 24819) 2-arg (non-triggered); 12s init,
-- randtime(10s,20s) repeat.
local function lightningWaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_LIGHTNING_WAVE)
    schedule(guid, "lightning_wave", {10000, 20000}, function()
        lightningWaveTick(creature, guid)
    end)
end

-- C++ boss_lethon::ExecuteEvent (:399-401): me->CastSpell(nullptr,
-- SPELL_SHADOW_BOLT_WHIRL 24834, false) — explicit non-triggered,
-- no explicit target; 10s init, randtime(15s,30s) repeat. Lowered to
-- self-cast creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE
-- self-cast convention — selin_fireheart / mr_smite precedent).
local function shadowBoltWhirlTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_BOLT_WHIRL)
    schedule(guid, "shadow_bolt_whirl", {15000, 30000}, function()
        shadowBoltWhirlTick(creature, guid)
    end)
end

-- C++ boss_emeriss::ExecuteEvent (:529-531): DoCastVictim(SPELL_
-- VOLATILE_INFECTION 24928) 2-arg (non-triggered); 12s init, 120s
-- repeat.
local function volatileInfectionTick(creature, guid)
    creature:CastSpell(nil, SPELL_VOLATILE_INFECTION)
    schedule(guid, "volatile_infection", 120000, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- C++ boss_taerar::ExecuteEvent (:649-651): DoCast(SPELL_ARCANE_BLAST
-- 24857) 1-arg (DoCastVictim); 12s init, randtime(7s,12s) repeat.
local function arcaneBlastTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARCANE_BLAST)
    schedule(guid, "arcane_blast", {7000, 12000}, function()
        arcaneBlastTick(creature, guid)
    end)
end

-- C++ boss_taerar::ExecuteEvent (:653-655): DoCast(SPELL_BELLOWING_
-- ROAR 22686) 1-arg. The 1-arg DoCast resolves via AISpellInfo
-- (UnitAI.cpp FillAISpellInfo): Bellowing Roar is a PBAOE with range
-- 0, so the target stays AITARGET_SELF — self-cast. 30s init,
-- randtime(20s,30s) repeat.
local function bellowingRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "bellowing_roar", {20000, 30000}, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::ExecuteEvent (:132-135): DoCast(me, SPELL_TAIL_
-- SWEEP 15847) 2-arg (non-triggered); 4s init, 2s repeat. All four
-- dragons fall through to the base ExecuteEvent for this event.
local function tailSweepTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tail_sweep", 2000, function()
        tailSweepTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::ExecuteEvent (:127-130): DoCast(me, SPELL_NOXIOUS_
-- BREATH 24818) 2-arg (non-triggered); randtime(7.5s,15s) init/repeat.
local function noxiousBreathTick(creature, guid)
    creature:CastSpell(creature, SPELL_NOXIOUS_BREATH)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::ExecuteEvent (:120-125): DoCast(me, SPELL_SEEPING_
-- FOG_LEFT 24813, triggered) + DoCast(me, SPELL_SEEPING_FOG_RIGHT
-- 24814, triggered); randtime(12.5s,20s) init, randtime(120s,150s)
-- repeat. The fog-pair casts are self-casts; the summoned dream-fog
-- creatures' own AI stays unported (motion machine, see below).
local function seepingFogTick(creature, guid)
    creature:CastSpell(creature, SPELL_SEEPING_FOG_LEFT, true)
    creature:CastSpell(creature, SPELL_SEEPING_FOG_RIGHT, true)
    schedule(guid, "seeping_fog", {120000, 150000}, function()
        seepingFogTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::UpdateAI (:158-159): SelectTarget(MaxThreat, 0,
-- -50.0f, playerOnly) -> DoCast(target, SPELL_SUMMON_PLAYER 24776)
-- 2-arg (non-triggered), every UpdateAI tick while the condition holds.
-- DefaultTargetSelector (UnitAI.cpp:259-279): a negative dist EXCLUDES
-- targets within 50yd — only the top-threat player BEYOND 50yd
-- (plus combat reaches) qualifies, i.e. a player wandering off.
-- Ported as a 1s combat pump (the per-tick check has no Lua event;
-- 1s is the file's pump granularity): iterate the threat-ordered
-- GetAITargets list (lua_creature_events.go:345), skipping
-- in-range players (C++ filters before picking), and cast when the
-- raw 3D GetDistance meets the C++
-- IsWithinCombatRange bar — exactDist >= 50 + dragon reach +
-- player reach, with the player reach taken as the 1.5 default
-- (combatReachOrDefault) since the player surface carries no
-- GetCombatReach read. C++ fires per tick; the 1s pump is coarser
-- but converges on the same teleport once the player stays out.
local function summonPlayerTick(creature, guid)
    local targets = creature:GetAITargets()
    if targets then
        -- C++ filters FIRST (DefaultTargetSelector drops in-range
        -- targets), then picks MaxThreat among survivors: skip
        -- in-range players, cast on the first player beyond the bar.
        for _, target in ipairs(targets) do
            if target:GetObjectType() == "Player" and not target:IsDead() then
                local bar = 50 + creature:GetCombatReach() + 1.5
                if creature:GetDistance(target) >= bar then
                    creature:CastSpell(target, SPELL_SUMMON_PLAYER)
                    break
                end
            end
        end
    end
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
end

-- C++ boss_ysondre::DamageTaken (:283-293): if (!HealthAbovePct(100 -
-- 25*_stage)) { Talk(1); 10x DoCast(me, 24795, triggered); ++_stage; }.
-- Pre-damage health in the event-9 hook (moroes precedent); C++-exact
-- floor form (ironaya convention); health > 0 hook guard (midnight).
local function ysondreDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local stage = stages[guid] or 1
    stages[guid] = stage
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:Talk(SAY_SUMMON)
        for _ = 1, 10 do
            creature:CastSpell(creature, SPELL_SUMMON_DRUID_SPIRITS, true)
        end
        stages[guid] = stage + 1
    end
end

-- C++ boss_lethon::DamageTaken (:376-384): if (!HealthAbovePct(100 -
-- 25*_stage)) { Talk(1); DoCast(me, 24811) non-triggered; ++_stage; }.
-- The SpellHitTarget spirit-shade summon is PORTED (lethonSpellHitTarget,
-- ev15 — both bridges now live); the draw-spirit cast itself is
-- visual-only (no Go effect for 24811).
local function lethonDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local stage = stages[guid] or 1
    stages[guid] = stage
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:Talk(SAY_SUMMON)
        creature:CastSpell(creature, SPELL_DRAW_SPIRIT)
        stages[guid] = stage + 1
    end
end

-- C++ boss_lethon::SpellHitTarget (boss_emerald_dragons.cpp:386-393):
-- if (spellInfo->Id == SPELL_DRAW_SPIRIT && target->GetTypeId() ==
-- TYPEID_PLAYER) { Position targetPos = target->GetPosition();
-- me->SummonCreature(NPC_SPIRIT_SHADE, targetPos,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 50s); } — gruul hook-15
-- precedent; Eluna summon type 3 = TIMED_DESPAWN_OUT_OF_COMBAT.
local function lethonSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_DRAW_SPIRIT and target and
            target:GetObjectType() == "Player" then
        creature:SummonCreature(ENTRY_SPIRIT_SHADE, target:GetX(), target:GetY(),
            target:GetZ(), target:GetO(), 3, 50000)
    end
end

-- C++ boss_emeriss::DamageTaken (:515-523): if (!HealthAbovePct(100 -
-- 25*_stage)) { Talk(1); DoCast(me, 24910, triggered); ++_stage; }.
local function emerissDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local stage = stages[guid] or 1
    stages[guid] = stage
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    if health <= math.floor(maxHealth * (100 - 25 * stage) / 100) then
        creature:Talk(SAY_SUMMON)
        creature:CastSpell(creature, SPELL_CORRUPTION_OF_EARTH, true)
        stages[guid] = stage + 1
    end
end

-- C++ boss_ysondre/boss_lethon/boss_emeriss/boss_taerar::JustEngagedWith:
-- Talk(SAY_*_AGGRO); WorldBossAI::JustEngagedWith(who) — the auriaya
-- precedent (the WorldBossAI legs have no bridges). Each dragon seeds
-- its own timer arm(s) below (the Oct-12 "no-timer bridge" claim was
-- wrong — CreateLuaEvent is a real bridge, retracted this run).
local function ysondreEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "lightning_wave", 12000, function()
        lightningWaveTick(creature, guid)
    end)
end

local function lethonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "shadow_bolt_whirl", 10000, function()
        shadowBoltWhirlTick(creature, guid)
    end)
end

local function emerissEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "volatile_infection", 12000, function()
        volatileInfectionTick(creature, guid)
    end)
end

local function taerarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "summon_player", 1000, function()
        summonPlayerTick(creature, guid)
    end)
    schedule(guid, "tail_sweep", 4000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "noxious_breath", {7500, 15000}, function()
        noxiousBreathTick(creature, guid)
    end)
    schedule(guid, "seeping_fog", {12500, 20000}, function()
        seepingFogTick(creature, guid)
    end)
    schedule(guid, "arcane_blast", 12000, function()
        arcaneBlastTick(creature, guid)
    end)
    schedule(guid, "bellowing_roar", 30000, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ emerald_dragonAI::KilledUnit (:109-113): player-gated
-- who->CastSpell(who, SPELL_MARK_OF_NATURE 25040, triggered). The C++ caster
-- is the victim self-cast; lowered to a triggered creature cast on the
-- victim (maiden_of_virtue / moroes triggered-cast convention) — the aura
-- lands on the same target. Mechanically inert in Go (no spell_mark_of_
-- nature effect), like the ported DamageTaken visual casts in this file.
local function dragonKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:CastSpell(victim, SPELL_MARK_OF_NATURE, true)
    end
end

-- C++ boss_emeriss::KilledUnit (:503-507): DoCast(who, SPELL_PUTRID_
-- MUSHROOM 24904, triggered) on the player victim, then the base
-- emerald_dragonAI::KilledUnit leg above.
local function emerissKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:CastSpell(victim, SPELL_PUTRID_MUSHROOM, true)
        creature:CastSpell(victim, SPELL_MARK_OF_NATURE, true)
    end
end

local function dragonLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function dragonDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function dragonReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    stages[guid] = 1 -- C++ Initialize()/Reset() sets _stage = 1
    -- C++ emerald_dragonAI::Reset (boss_emerald_dragons.cpp:100):
    -- RemoveFlag(UNIT_FIELD_FLAGS, NOT_SELECTABLE|NON_ATTACKABLE)
    -- (ported 2026-10-20); the banish-machine SetFlag pair (:638) is
    -- unported — the flags now bridge but the pair rides the unported
    -- banish machine (no SummonedCreatureDies / InterruptNonMeleeSpells
    -- bridges), so this is the only flag leg at a registered site.
    -- C++ :101's SetReactState(REACT_AGGRESSIVE) stays out: nothing in
    -- the ported model ever sets the dragons non-aggressive (the
    -- 25%-stage banish machine's REACT_PASSIVE :639 and its
    -- UpdateAI-timeout AGGRESSIVE restore :677 are both unported),
    -- and spawn default is AGGRESSIVE.
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_NON_ATTACKABLE)
    -- C++ emerald_dragonAI::Reset (:102): DoCast(me, SPELL_MARK_OF_NATURE_
    -- AURA 25041, triggered) — ported for C++-exactness; mechanically
    -- inert in Go (its 10s tick triggers 25042 via the SpellScript-only
    -- spell_mark_of_nature), like the ported KilledUnit 25040 cast.
    creature:CastSpell(creature, SPELL_MARK_OF_NATURE_AURA, true)
end

-- C++ boss_taerarAI::Reset (boss_emerald_dragons.cpp:596-600):
-- RemoveAurasDueToSpell(SPELL_SHADE) then Initialize(); it does NOT call
-- the base emerald_dragonAI::Reset. The apply leg (DoCast 24313 in the
-- unported 25%-stage banish machine) never fires in Lua, so the removal
-- is currently a no-op — kept C++-exact; the base Reset's RemoveFlag
-- (NOT_SELECTABLE/NON_ATTACKABLE) legs are PORTED in dragonReset
-- above (2026-10-20).
local function taerarReset(event, creature)
    dragonReset(event, creature)
    creature:RemoveAura(SPELL_SHADE)
end

RegisterCreatureEvent(ENTRY_YSONDRE, 1, ysondreEnterCombat)
RegisterCreatureEvent(ENTRY_YSONDRE, 2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_YSONDRE, 4, dragonDied)
RegisterCreatureEvent(ENTRY_YSONDRE, 3, dragonKilledUnit)
RegisterCreatureEvent(ENTRY_YSONDRE, 9, ysondreDamageTaken)
RegisterCreatureEvent(ENTRY_YSONDRE, 23, dragonReset)
RegisterCreatureEvent(ENTRY_LETHON,  1, lethonEnterCombat)
RegisterCreatureEvent(ENTRY_LETHON,  2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_LETHON,  4, dragonDied)
RegisterCreatureEvent(ENTRY_LETHON,  3, dragonKilledUnit)
RegisterCreatureEvent(ENTRY_LETHON,  9, lethonDamageTaken)
RegisterCreatureEvent(ENTRY_LETHON,  15, lethonSpellHitTarget)
RegisterCreatureEvent(ENTRY_LETHON,  23, dragonReset)
RegisterCreatureEvent(ENTRY_EMERISS, 1, emerissEnterCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 4, dragonDied)
RegisterCreatureEvent(ENTRY_EMERISS, 3, emerissKilledUnit)
RegisterCreatureEvent(ENTRY_EMERISS, 9, emerissDamageTaken)
RegisterCreatureEvent(ENTRY_EMERISS, 23, dragonReset)
RegisterCreatureEvent(ENTRY_TAERAR,  1, taerarEnterCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  4, dragonDied)
RegisterCreatureEvent(ENTRY_TAERAR,  3, dragonKilledUnit)
RegisterCreatureEvent(ENTRY_TAERAR,  23, taerarReset)
