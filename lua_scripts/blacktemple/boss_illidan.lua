-- Illidan Stormrage (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_illidan.cpp
-- (boss_illidan_stormrage, npc_akama_illidan, npc_flame_of_azzinoth,
-- npc_illidan_db_target, npc_maiev, npc_blade_of_azzinoth,
-- npc_illidan_generic_fire; npc_parasitic_shadowfiend,
-- npc_illidari_elite, npc_shadow_demon, npc_cage_trap_trigger and
-- the twenty-two spell/aura scripts documented below, not
-- registered); black_temple.h:39 (DATA_ILLIDAN_STORMRAGE = 8, ninth
-- and final boss), :58 (DATA_ILLIDAN_MUSIC_CONTROLLER = 23), :79
-- (NPC_ILLIDAN_STORMRAGE = 22917), :87 (NPC_AKAMA = 23089),
-- :100-:101 (NPC_SPIRIT_OF_UDALO = 23410, NPC_SPIRIT_OF_OLUM =
-- 23411), :102 (NPC_FLAME_OF_AZZINOTH = 22997), :103 (NPC_BLADE_OF_
-- AZZINOTH = 22996), :104 (NPC_MAIEV_SHADOWSONG = 23197), :105
-- (NPC_ILLIDAN_DB_TARGET = 23070), :106 (NPC_ILLIDARI_ELITE =
-- 23226), :109 (NPC_DEMON_FIRE = 23069), :110 (NPC_PARASITIC_
-- SHADOWFIEND = 23498), :111 (NPC_BLAZE = 23259), :112 (NPC_FLAME_
-- CRASH = 23336).
-- Creature entries: 22917 Illidan Stormrage (C++ ScriptName
-- "boss_illidan_stormrage"), 23089 Akama (C++ ScriptName
-- "npc_akama_illidan"), 22997 Flame of Azzinoth (C++ ScriptName
-- "npc_flame_of_azzinoth"), 23070 Illidan DB Target (C++ ScriptName
-- "npc_illidan_db_target"), 23197 Maiev Shadowsong (C++ ScriptName
-- "npc_maiev"), 22996 Blade of Azzinoth (C++ ScriptName
-- "npc_blade_of_azzinoth"), 23069 Demon Fire / 23259 Blaze / 23336
-- Flame Crash (C++ ScriptName "npc_illidan_generic_fire", entry
-- switch), all per RegisterBlackTempleCreatureAI in
-- AddSC_boss_illidan; the creature_template ScriptName bindings
-- are DB-side — no TDB in this workspace, so only the C++-side
-- naming is verified. Talk lines used: Illidan SAY_ILLIDAN_MINION=0
-- (minions), SAY_ILLIDAN_KILL=1 (kill, player-only), SAY_ILLIDAN_
-- EYE_BLAST=4 (eye blast), SAY_ILLIDAN_MORPH=5 (demon), SAY_ILLIDAN_
-- ENRAGE=6 (berserk), SAY_ILLIDAN_TAUNT=7 (taunt), SAY_ILLIDAN_
-- SHADOW_PRISON=11 (shadow prison), SAY_ILLIDAN_CONFRONT_MAIEV=12
-- (maiev), SAY_ILLIDAN_FRENZY=13 (frenzy), SAY_ILLIDAN_DEFEATED=14
-- (defeated); the intro lines SAY_ILLIDAN_DUPLICITY=8, SAY_ILLIDAN_
-- UNCONVINCED=9, SAY_ILLIDAN_PREPARED=10, SAY_ILLIDAN_TAKEOFF=2 and
-- SAY_ILLIDAN_SUMMONFLAMES=3 belong to the unmodeled intro/air
-- movement arms. Maiev SAY_MAIEV_SHADOWSONG_TAUNT=0 (taunt),
-- SAY_MAIEV_SHADOWSONG_TRAP=3 (cage trap), SAY_MAIEV_SHADOWSONG_
-- DOWN=4 (down); the appear/justice/outro/farewell lines belong to
-- the unmodeled summon/intro/outro arms.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (returns false,
-- newDamage; the boolean is consumed by the engine, the number
-- rewrites damage — reliquary convention), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): Illidan (22917).
-- OnEnterCombat (the intro arms — PHASE_INTRO, the akama gossip
-- chain, SetImmuneToAll, DoZoneInCombat and the instance-data
-- gates — have no gossip/instance bridges, so the fight starts on
-- pull, ragnaros-intro convention): per-GUID state reset
-- (phase=PHASE_1, _dead=false, _isDemon=false; C++ Reset values),
-- berserk 45078 25min one-shot, triggered self-cast (C++
-- DoCastSelf(..., true)) + Talk(SAY_ILLIDAN_ENRAGE) / phase-1
-- schedule: flame crash 40832 30s then 30s, non-triggered
-- DoCastVictim (nil ticks cast nothing but keep the schedule,
-- jeklik convention) / draw soul 40904 34s then 34s,
-- non-triggered self-cast (C++ DoCastAOE resolves to the caster's
-- own position — vaelastrasz convention) / shear 41032 10s then
-- 12s, non-triggered DoCastVictim / parasitic shadowfiend 41917
-- 26s then 30s, non-triggered on a random alive player in the
-- instance (C++ SelectTarget(Random, 0) — teron convention) /
-- taunt Talk(SAY_ILLIDAN_TAUNT) 30s then {30s,60s}. The 10s evade
-- check has no boundary bridge — skipped; the music controller
-- arms have no sound bridge; SetCanDualWield has no bridge.
-- OnDamageTaken(9, pre-damage hook): lethal from anyone but self
-- (C++-exact guard) -> damage rewritten to health-1; if !_dead:
-- _isDemon -> cancel all timers, non-triggered self-cast demon
-- transform 40511 (C++-exact: events/specialEvents reset,
-- DoCastSelf default triggered=false) / else _dead=true, cancel
-- all timers, triggered self-cast remove-parasitic 41923 (the
-- parasitic summon-despawn arm has no summon bridge) +
-- ACTION_START_OUTRO: triggered self-cast death 41218 (the
-- NOT_SELECTABLE arm has no flag bridge), Talk(SAY_ILLIDAN_
-- DEFEATED) 4s, triggered self-cast quiet suicide 3617 18s later;
-- the maiev DoAction(ACTION_START_OUTRO) and the summon-despawn
-- arms have no cross-creature/summon bridges — skipped. 90% health
-- & phase<PHASE_MINIONS: Talk(SAY_ILLIDAN_MINION) + the 30s
-- minions-weave cycle; the weave fires empty (no summon bridge —
-- hexlord convention) and the akama DoAction(ACTION_START_MINIONS)
-- relay has no cross-creature bridge — skipped. 65% & phase<
-- PHASE_2: the phase-1 timers are canceled and the air-phase timer
-- chain starts (fly 1s -> warglaive point 6s -> throw warglaive
-- 39849 2s, non-triggered self-cast -> throw warglaive 2 39635 1s,
-- non-triggered self-cast -> pillar 2s then 30s: cancel the phase-2
-- timers, random pillar index, arm fireball 40598 {1s,8s} then
-- {2s,4s} non-triggered on a random alive player in the instance
-- within 150 yd (C++ SelectTarget(Random, 0, 150.0f, true)) / eye
-- blast {1s,30s} one-shot: Talk(SAY_ILLIDAN_EYE_BLAST) — the db-
-- target summon and the 39908 cast on it have no summon bridge,
-- skipped / dark barrage 40585 50% {1s,20s} one-shot, non-
-- triggered on a random alive player within 150 yd, then re-arm
-- eye blast 5s and re-arm the pillar timer 30s (C++ adds 30s to
-- the pending pillar time — approximated, documented); the liftoff
-- emote/sound/gravity arms, the warglaive-point movement and the
-- pillar MovePoints have no movement bridge — the chain keeps the
-- timers, the positions are unmodeled; the two flame-of-azzinoth
-- summons have no summon bridge, so the both-flames-dead finalize
-- arm (middle move, glaive returns, land, resume-combat phase 3)
-- has no bearer and the air phase loops on the pillar timer —
-- documented. 30% & phase<PHASE_4: _isDemon -> _isDemon=false,
-- cancel the demon-spell timers, triggered self-cast 40511, phase-4
-- delayed 12s (the root/interrupt arms have no bridge; the pending
-- 72s cancel-demon timer is kept, C++-exact) / else start phase 4:
-- cancel the phase-1/2/3 timers, triggered self-cast shadow prison
-- 40647 (the react/flag and summon-DoAction arms have no bridges),
-- Talk(SAY_ILLIDAN_SHADOW_PRISON) 500ms, non-triggered self-cast
-- summon maiev 40403 9s (the summon itself has no bridge), Talk
-- (SAY_ILLIDAN_CONFRONT_MAIEV) 9s later, resume combat 13s later:
-- arm the phase-4 schedule (phase-1 arms + agonizing flames 40834
-- 21s then 53s, non-triggered self-cast + frenzy 40683 40s then
-- 40s, non-triggered self-cast + Talk(SAY_ILLIDAN_FRENZY) + the
-- 60s one-shot demon arm: _isDemon=true, cancel the phase
-- schedule (berserk/taunt/weave survive, C++-exact group scope),
-- triggered self-cast demon transform 40511, Talk(SAY_ILLIDAN_
-- MORPH) 2s, demon spells 15s (shadow blast 41078 1s then 2s,
-- non-triggered DoCastVictim / flame burst 41126 6s then 22s,
-- non-triggered self-cast / summon shadowdemon 41117 {18s,30s}
-- one-shot, non-triggered self-cast — the summon has no bridge,
-- cast kept), cancel demon form 72s one-shot: _isDemon=false,
-- cancel the demon-spell timers, triggered self-cast 40511,
-- resume-demon 12s re-arms the phase schedule (the ResetThreatList
-- and equipment arms have no bridges)). OnTargetDied(3):
-- Talk(SAY_ILLIDAN_KILL), IsPlayer gate (terestian convention).
-- OnDied(4)/OnLeaveCombat(2)/OnReset(23): cancel timers, drop
-- per-GUID state (the Reset summon-group/equipment/sheath/root/
-- gravity arms and the akama-intro DoAction have no bridges; the
-- JustDied instance SetBossState arm is blocked on the
-- instance-script model).
-- Akama (23089): OnEnterCombat arms the healing-potion loop 1s:
-- health strictly below 20% -> non-triggered self-cast 40535
-- (C++-exact). OnDamageTaken(9): lethal damage rewritten to
-- health-1 (C++-exact — no self-exclusion in the C++). The entire
-- intro arc (gossip chains, spline movement, door channel 41268,
-- door fail 41271, spirit summons 23410/23411, teleport 41077),
-- the minions-phase arms (EVENT_AKAMA_MINIONS chain driven by
-- illidan's DoAction(ACTION_START_MINIONS)) and the outro arms
-- (DoAction(ACTION_START_OUTRO) from illidan's quiet-suicide)
-- have no gossip/movement/instance/summon/DoAction bridges —
-- unmodeled; the CanAIAttack illidan gate has no attack-gating
-- bridge. OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel timers.
-- Flame of Azzinoth (22997): OnReset: triggered self-cast flame
-- tear 39856 (C++ DoCastSelf(..., true); the boss-state despawn
-- check is blocked on the instance-script model; REACT_PASSIVE
-- has no bridge). OnEnterCombat: engage 3s (the react/zone-
-- combat arms have no bridges), then charge 42003 5s on a random
-- alive player in the instance more than 25 yd (2D) from both
-- blade positions (676.226,325.231) and (678.060,285.220)
-- (C++ ChargeTargetSelector, C++-exact), repeat 5s with a target
-- else 1s (C++-exact) / flame blast 40631 11s then 12s,
-- non-triggered self-cast (C++ DoCastAOE). The JustDied
-- ACTION_FLAME_DEAD relay to illidan has no cross-creature
-- bridge — the air-phase finalize arm stays unmodeled.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel timers.
-- Illidan DB target (23070): OnReset: non-triggered self-cast eye
-- blast trigger 40017 (C++ DoCastSelf default is triggered=false —
-- vaelastrasz convention; the MovementInform aura-removal arm has
-- no movement bridge).
-- Maiev Shadowsong (23197): OnEnterCombat arms cage trap 40694
-- 30s then 30s, non-triggered self-cast + Talk(SAY_MAIEV_
-- SHADOWSONG_TRAP) — the illidan->CastSpell(CAGED_TRAP_TELEPORT
-- 40693) arm has no cross-creature bridge, skipped / shadow
-- strike 40685 50s then 50s, non-triggered DoCastVictim / throw
-- dagger 41152 1s: victim beyond melee range -> non-triggered
-- DoCastVictim, repeat 5s, else repeat 1s (melee range
-- approximated at 5 yd — documented) / taunt Talk(SAY_MAIEV_
-- SHADOWSONG_TAUNT) {20s,60s} then {30s,60s}. OnDamageTaken(9):
-- lethal while _canDown -> health-1, triggered self-cast maiev
-- down 40409 + Talk(SAY_MAIEV_SHADOWSONG_DOWN), _canDown=false;
-- the ACTION_MAIEV_DOWN_FADE re-arm lives in the unmodeled maiev-
-- down AuraScript (standing AuraScript gap). The appear sequence
-- (summon-driven Talk(SAY_MAIEV_SHADOWSONG_APPEAR)), the outro
-- chain (ACTION_START_OUTRO from illidan) and the teleport-
-- despawn have no summon/DoAction/despawn bridges — unmodeled;
-- the emote arms have no emote bridge. OnLeaveCombat(2)/OnDied(4)
-- /OnReset(23): cancel timers, drop per-GUID state.
-- Blade of Azzinoth (22996): OnReset: triggered self-cast birth
-- 40031 (C++ DoCastSelf(..., true); the spawn sound, the tear-of-
-- azzinoth summon chain and the boss-state despawn check have no
-- sound/summon/instance bridges — skipped).
-- Illidan generic fire (23069 demon fire / 23259 blaze / 23336
-- flame crash): OnReset entry switch, all triggered self-casts
-- (C++-exact): 40029 demon fire / 40610 blaze + 40031 birth /
-- 40836 flame crash ground.
-- Not registered (entries verifiable but no modelable arm without
-- engine bridges): npc_parasitic_shadowfiend (23498 — Reset is the
-- instance boss-state check + react/zone-combat arms, all
-- bridgeless; UpdateAI is melee only), npc_illidari_elite (23226
-- — Reset is AttackStart/AddThreat on akama, CanAIAttack gates
-- illidan; no threat/attack-gating bridges). Not registered
-- (entries not verifiable from the C++ sources — the ScriptNames
-- bind DB-side, garr firesworn convention): npc_shadow_demon,
-- npc_cage_trap_trigger. Not registered (standing script gaps):
-- the twenty-two SpellScripts/AuraScripts (akama teleport, akama
-- door channel, draw soul, parasitic shadowfiend x3, throw
-- warglaive, tear of azzinoth channel, flame blast, return
-- glaives, agonizing flames, demon transform x2, flame burst, find
-- target, eye blast, cage trap, caged, maiev down, cage teleport,
-- despawn akama).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim (BossAI::
-- JustEngagedWith/JustDied/Reset, the DATA_ILLIDAN_STORMRAGE /
-- DATA_AKAMA / DATA_MAIEV / DATA_ILLIDAN_MUSIC_CONTROLLER
-- bookkeeping arms and every instance->GetCreature DoAction relay
-- skipped); no movement bridge — the illidan liftoff/pillar/
-- glaive-point/middle MovePoints and the akama spline chains are
-- unmodeled (the illidan air-phase timer chain is kept, the
-- positions are not); no summon bridge — the warglaive/db-target/
-- maiev/shadow-demon/cage-trap/blade-tear/spirit/minion summons
-- never spawn; no gossip bridge — the akama intro never starts;
-- no flag/react/equipment/sheath/gravity/threat/immune/sound/
-- despawn bridges — those arms are skipped; the air-phase
-- finalize arm (both flames dead -> middle, glaive returns, land,
-- phase 3) has no bearer, so the air phase loops on the pillar
-- timer and phase 3 only ever starts via the resume-demon path;
-- the 30s dark-barrage pillar extension is approximated by
-- re-arming the pillar timer 30s from the barrage tick.

local ENTRY_ILLIDAN = 22917
local ENTRY_AKAMA = 23089
local ENTRY_FLAME_OF_AZZINOTH = 22997
local ENTRY_DB_TARGET = 23070
local ENTRY_MAIEV = 23197
local ENTRY_BLADE_OF_AZZINOTH = 22996
local ENTRY_DEMON_FIRE = 23069
local ENTRY_BLAZE = 23259
local ENTRY_FLAME_CRASH = 23336

-- Illidan phases (C++ IllidanPhases; PHASE_INTRO is unmodeled).
local PHASE_1 = 2
local PHASE_MINIONS = 3
local PHASE_2 = 4
local PHASE_3 = 5
local PHASE_4 = 6

-- Illidan talk lines (modeled arms only).
local SAY_ILLIDAN_MINION = 0
local SAY_ILLIDAN_KILL = 1
local SAY_ILLIDAN_EYE_BLAST = 4
local SAY_ILLIDAN_MORPH = 5
local SAY_ILLIDAN_ENRAGE = 6
local SAY_ILLIDAN_TAUNT = 7
local SAY_ILLIDAN_SHADOW_PRISON = 11
local SAY_ILLIDAN_CONFRONT_MAIEV = 12
local SAY_ILLIDAN_FRENZY = 13
local SAY_ILLIDAN_DEFEATED = 14

-- Maiev talk lines (modeled arms only).
local SAY_MAIEV_TAUNT = 0
local SAY_MAIEV_TRAP = 3
local SAY_MAIEV_DOWN = 4

-- Illidan spells.
local SPELL_FLAME_CRASH = 40832
local SPELL_DRAW_SOUL = 40904
local SPELL_SHEAR = 41032
local SPELL_PARASITIC_SHADOWFIEND = 41917
local SPELL_BERSERK = 45078
local SPELL_AGONIZING_FLAMES_SELECTOR = 40834
local SPELL_DEMON_TRANSFORM_1 = 40511
local SPELL_THROW_GLAIVE = 39849
local SPELL_THROW_GLAIVE2 = 39635
local SPELL_FIREBALL = 40598
local SPELL_DARK_BARRAGE = 40585
local SPELL_SHADOW_BLAST = 41078
local SPELL_FLAME_BURST = 41126
local SPELL_SUMMON_SHADOWDEMON = 41117
local SPELL_FRENZY = 40683
local SPELL_SHADOW_PRISON = 40647
local SPELL_SUMMON_MAIEV = 40403
local SPELL_REMOVE_PARASITIC_SHADOWFIEND = 41923
local SPELL_DEATH = 41218
local SPELL_QUIET_SUICIDE = 3617

-- Flame of Azzinoth spells.
local SPELL_FLAME_TEAR_OF_AZZINOTH = 39856
local SPELL_CHARGE = 42003
local SPELL_FLAME_BLAST = 40631

-- Blade of Azzinoth / generic fire / db target / maiev / akama spells.
local SPELL_BIRTH = 40031
local SPELL_EYE_BLAST_TRIGGER = 40017
local SPELL_DEMON_FIRE = 40029
local SPELL_BLAZE = 40610
local SPELL_FLAME_CRASH_GROUND = 40836
local SPELL_CAGE_TRAP_SUMMON = 40694
local SPELL_SHADOW_STRIKE = 40685
local SPELL_THROW_DAGGER = 41152
local SPELL_MAIEV_DOWN = 40409
local SPELL_HEALING_POTION = 40535

-- C++ BladesPositions: the two warglaive landing spots the flame
-- charge target must stay 25 yd (2D) away from.
local BLADE_X1, BLADE_Y1 = 676.226013, 325.230988
local BLADE_X2, BLADE_Y2 = 678.059998, 285.220001

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

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
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

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0): any alive player in the instance,
-- no range cap, victim included.
local function randomAnyTarget(creature)
    local candidates = playersInInstance(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 0, 150.0f, true): any alive player in
-- the instance within 150 yd.
local function randomTargetIn150(creature)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= 150 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

--
-- Illidan Stormrage (22917)
--

local illidanState = {}

local function getIllidanState(guid)
    local state = illidanState[guid]
    if state == nil then
        state = { phase = PHASE_1, dead = false, isDemon = false,
                  pillarIndex = 0 }
        illidanState[guid] = state
    end
    return state
end

-- C++ ScheduleEvents(GROUP_PHASE_1, group): the first-fire times
-- are 30s/34s/10s/26s.
local function armPhase1(creature, guid)
    schedule(guid, "flameCrash", 30000, function()
        onFlameCrash(creature, guid)
    end)
    schedule(guid, "drawSoul", 34000, function()
        onDrawSoul(creature, guid)
    end)
    schedule(guid, "shear", 10000, function()
        onShear(creature, guid)
    end)
    schedule(guid, "parasitic", 26000, function()
        onParasitic(creature, guid)
    end)
end

-- C++ EVENT_DEMON cancels GROUP_PHASE_3 or GROUP_PHASE_4: the
-- phase-schedule timers, not berserk/taunt/weave (GROUP_PHASE_ALL)
-- and not the specialEvents timers (berserk/cancel-demon).
local function cancelPhaseSchedule(guid)
    for _, key in ipairs({ "flameCrash", "drawSoul", "shear",
            "parasitic", "agonizing", "frenzy", "demon",
            "shadowBlast", "flameBurst", "shadowDemon",
            "resumeDemon", "demonText", "demonSpells" }) do
        cancelTimer(guid, key)
    end
end

local function cancelAirPhase(guid)
    for _, key in ipairs({ "fly", "warglaivePoint", "throwGlaive",
            "throwGlaive2", "pillar", "fireball", "eyeBlast",
            "darkBarrage" }) do
        cancelTimer(guid, key)
    end
end

-- C++ EVENT_BERSERK (specialEvents, 25min one-shot): Talk +
-- triggered self-cast.
local function onBerserk(creature, guid)
    creature:Talk(SAY_ILLIDAN_ENRAGE)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

-- C++ EVENT_TAUNT: Talk; re-arm {30s,60s} unconditional.
local function onTaunt(creature, guid)
    creature:Talk(SAY_ILLIDAN_TAUNT)
    schedule(guid, "taunt", 30000 + math.random(0, 30000), function()
        onTaunt(creature, guid)
    end)
end

-- C++ EVENT_FLAME_CRASH: non-triggered DoCastVictim; repeat 30s.
local function onFlameCrash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAME_CRASH, false)
    end
    schedule(guid, "flameCrash", 30000, function()
        onFlameCrash(creature, guid)
    end)
end

-- C++ EVENT_DRAW_SOUL: non-triggered DoCastAOE (caster's own
-- position — self-targeted bridge); repeat 34s.
local function onDrawSoul(creature, guid)
    creature:CastSpell(creature, SPELL_DRAW_SOUL, false)
    schedule(guid, "drawSoul", 34000, function()
        onDrawSoul(creature, guid)
    end)
end

-- C++ EVENT_SHEAR: non-triggered DoCastVictim; repeat 12s.
local function onShear(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHEAR, false)
    end
    schedule(guid, "shear", 12000, function()
        onShear(creature, guid)
    end)
end

-- C++ EVENT_PARASITIC_SHADOWFIEND: non-triggered 41917 on a
-- random target; repeat 30s.
local function onParasitic(creature, guid)
    local target = randomAnyTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_PARASITIC_SHADOWFIEND, false)
    end
    schedule(guid, "parasitic", 30000, function()
        onParasitic(creature, guid)
    end)
end

-- C++ EVENT_MINIONS_WEAVE: _dead guard, SummonMinions (no summon
-- bridge — fires empty, hexlord convention), repeat 30s.
local function onMinionsWeave(creature, guid)
    local state = getIllidanState(guid)
    if state.dead then
        return
    end
    schedule(guid, "minionsWeave", 30000, function()
        onMinionsWeave(creature, guid)
    end)
end

-- C++ EVENT_AGONIZING_FLAMES: non-triggered self-cast; repeat 53s.
local function onAgonizing(creature, guid)
    creature:CastSpell(creature, SPELL_AGONIZING_FLAMES_SELECTOR, false)
    schedule(guid, "agonizing", 53000, function()
        onAgonizing(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: non-triggered self-cast + Talk; repeat 40s.
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY, false)
    creature:Talk(SAY_ILLIDAN_FRENZY)
    schedule(guid, "frenzy", 40000, function()
        onFrenzy(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BLAST: non-triggered DoCastVictim; repeat 2s.
local function onShadowBlast(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BLAST, false)
    end
    schedule(guid, "shadowBlast", 2000, function()
        onShadowBlast(creature, guid)
    end)
end

-- C++ EVENT_FLAME_BURST: non-triggered self-cast; repeat 22s.
local function onFlameBurst(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_BURST, false)
    schedule(guid, "flameBurst", 22000, function()
        onFlameBurst(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_DEMON: non-triggered self-cast (the summon has
-- no bridge — cast kept); one-shot.
local function onShadowDemon(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_SHADOWDEMON, false)
end

-- C++ EVENT_SCHEDULE_DEMON_SPELLS: GROUP_PHASE_DEMON arms.
local function onDemonSpells(creature, guid)
    schedule(guid, "shadowBlast", 1000, function()
        onShadowBlast(creature, guid)
    end)
    schedule(guid, "flameBurst", 6000, function()
        onFlameBurst(creature, guid)
    end)
    schedule(guid, "shadowDemon", 18000 + math.random(0, 12000),
        function()
            onShadowDemon(creature, guid)
        end)
end

-- C++ armPhaseSchedule helper: GROUP_PHASE_3 = phase-1 arms +
-- agonizing + the 60s one-shot demon arm; GROUP_PHASE_4 adds
-- frenzy.
local function armPhaseSchedule(creature, guid, phase)
    armPhase1(creature, guid)
    if phase >= PHASE_3 then
        schedule(guid, "agonizing", 21000, function()
            onAgonizing(creature, guid)
        end)
        schedule(guid, "demon", 60000, function()
            onDemon(creature, guid)
        end)
    end
    if phase == PHASE_4 then
        schedule(guid, "frenzy", 40000, function()
            onFrenzy(creature, guid)
        end)
    end
end

-- C++ EVENT_CANCEL_DEMON_FORM (specialEvents, 72s one-shot):
-- _isDemon=false, triggered self-cast 40511, resume-demon 12s
-- re-arms the phase schedule (C++-exact; the interrupt/root arms
-- have no bridges).
local function onCancelDemon(creature, guid)
    local state = getIllidanState(guid)
    state.isDemon = false
    for _, key in ipairs({ "shadowBlast", "flameBurst",
            "shadowDemon" }) do
        cancelTimer(guid, key)
    end
    creature:CastSpell(creature, SPELL_DEMON_TRANSFORM_1, true)
    schedule(guid, "resumeDemon", 12000, function()
        armPhaseSchedule(creature, guid, state.phase)
    end)
end

-- C++ EVENT_DEMON (60s one-shot): _isDemon=true, cancel the phase
-- schedule, triggered self-cast 40511, demon text 2s, demon
-- spells 15s, cancel-demon 72s (the root/equipment arms have no
-- bridges).
local function onDemon(creature, guid)
    local state = getIllidanState(guid)
    state.isDemon = true
    cancelPhaseSchedule(guid)
    creature:CastSpell(creature, SPELL_DEMON_TRANSFORM_1, true)
    schedule(guid, "demonText", 2000, function()
        creature:Talk(SAY_ILLIDAN_MORPH)
    end)
    schedule(guid, "demonSpells", 15000, function()
        onDemonSpells(creature, guid)
    end)
    schedule(guid, "cancelDemon", 72000, function()
        onCancelDemon(creature, guid)
    end)
end

-- C++ EVENT_FIREBALL: non-triggered 40598 on a random alive player
-- within 150 yd; repeat {2s,4s}.
local function onFireball(creature, guid)
    local target = randomTargetIn150(creature)
    if target then
        creature:CastSpell(target, SPELL_FIREBALL, false)
    end
    schedule(guid, "fireball", 2000 + math.random(0, 2000), function()
        onFireball(creature, guid)
    end)
end

-- C++ EVENT_EYE_BLAST: cancels the pending dark barrage, Talk;
-- the db-target summon and the 39908 cast on it have no summon
-- bridge — skipped; one-shot.
local function onEyeBlast(creature, guid)
    cancelTimer(guid, "darkBarrage")
    creature:Talk(SAY_ILLIDAN_EYE_BLAST)
end

-- C++ EVENT_DARK_BARRAGE: non-triggered 40585 on a random alive
-- player within 150 yd; re-arms eye blast 5s and extends the
-- pillar timer by 30s (approximated by re-arming the pillar timer
-- 30s from this tick — documented); one-shot.
local function onDarkBarrage(creature, guid)
    local target = randomTargetIn150(creature)
    if target then
        creature:CastSpell(target, SPELL_DARK_BARRAGE, false)
    end
    schedule(guid, "eyeBlast", 5000, function()
        onEyeBlast(creature, guid)
    end)
    schedule(guid, "pillar", 30000, function()
        onPillar(creature, guid)
    end)
end

-- C++ EVENT_FLY_TO_RANDOM_PILLAR (30s cycle): cancel the phase-2
-- timers, random pillar index, arm the phase-2 casts (the MovePoint
-- and the MovementInform->ScheduleEvents arms have no movement
-- bridge — the chain keeps the timers, the positions are
-- unmodeled).
local function onPillar(creature, guid)
    local state = getIllidanState(guid)
    for _, key in ipairs({ "fireball", "eyeBlast",
            "darkBarrage" }) do
        cancelTimer(guid, key)
    end
    state.pillarIndex = math.random(0, 3)
    schedule(guid, "fireball", 1000 + math.random(0, 7000),
        function()
            onFireball(creature, guid)
        end)
    schedule(guid, "eyeBlast", 1000 + math.random(0, 29000),
        function()
            onEyeBlast(creature, guid)
        end)
    if math.random(2) == 1 then
        schedule(guid, "darkBarrage", 1000 + math.random(0, 19000),
            function()
                onDarkBarrage(creature, guid)
            end)
    end
    schedule(guid, "pillar", 30000, function()
        onPillar(creature, guid)
    end)
end

-- C++ EVENT_THROW_WARGLAIVE_2: non-triggered self-cast 39635,
-- then the pillar timer 2s (the sheath arm has no bridge).
local function onThrowGlaive2(creature, guid)
    creature:CastSpell(creature, SPELL_THROW_GLAIVE2, false)
    schedule(guid, "pillar", 2000, function()
        onPillar(creature, guid)
    end)
end

-- C++ EVENT_THROW_WARGLAIVE: non-triggered self-cast 39849, then
-- throw glaive 2 in 1s.
local function onThrowGlaive(creature, guid)
    creature:CastSpell(creature, SPELL_THROW_GLAIVE, false)
    schedule(guid, "throwGlaive2", 1000, function()
        onThrowGlaive2(creature, guid)
    end)
end

-- C++ EVENT_MOVE_TO_WARGLAIVE_POINT (6s): the glaive-trigger scan
-- and the MovePoint have no bridges — the timer chain is kept,
-- the movement is unmodeled; then throw glaive 2s (the
-- POINT_THROW_GLAIVE MovementInform arms: sound — no bridge).
local function onWarglaivePoint(creature, guid)
    schedule(guid, "throwGlaive", 2000, function()
        onThrowGlaive(creature, guid)
    end)
end

-- C++ EVENT_FLY (1s): the orientation arm has no facing bridge —
-- kept as a timer step; then the warglaive point 6s.
local function onFly(creature, guid)
    schedule(guid, "warglaivePoint", 6000, function()
        onWarglaivePoint(creature, guid)
    end)
end

-- C++ DoAction(ACTION_START_PHASE_4): cancel GROUP_PHASE_3 (here
-- the armed phase-1/2/3 timers — the air-phase cancel is a
-- documented extension since the finalize arm has no bearer),
-- triggered self-cast shadow prison 40647, Talk 500ms, summon
-- maiev (cast kept, summon unmodeled) 9s, confront text 9s,
-- resume combat 13s re-arming the phase-4 schedule. The react/
-- flag and summon-DoAction arms have no bridges.
local function startPhase4(creature, guid)
    cancelPhaseSchedule(guid)
    -- The air-phase timers are canceled here too: the both-flames-
    -- dead finalize arm has no bearer, so without this the pillar
    -- loop would run under the phase-4 schedule (documented
    -- extension of the C++ GROUP_PHASE_3 cancel). The minions weave
    -- (GROUP_PHASE_ALL) survives, C++-exact.
    cancelAirPhase(guid)
    creature:CastSpell(creature, SPELL_SHADOW_PRISON, true)
    schedule(guid, "shadowPrisonText", 500, function()
        creature:Talk(SAY_ILLIDAN_SHADOW_PRISON)
        schedule(guid, "summonMaiev", 9000, function()
            creature:CastSpell(creature, SPELL_SUMMON_MAIEV, false)
            schedule(guid, "confrontMaiev", 9000, function()
                creature:Talk(SAY_ILLIDAN_CONFRONT_MAIEV)
                schedule(guid, "resumeCombat4", 13000, function()
                    armPhaseSchedule(creature, guid, PHASE_4)
                end)
            end)
        end)
    end)
end

-- C++ JustEngagedWith: the intro/immunity/music/dual-wield arms
-- have no bridges; the fight starts on pull (ragnaros-intro
-- convention). Berserk 25min one-shot, phase-1 schedule, taunt
-- {30s,60s}. The 10s evade check has no boundary bridge.
local function illidanEnterCombat(event, creature)
    local guid = creature:GetGUID()
    illidanState[guid] = { phase = PHASE_1, dead = false,
                           isDemon = false, pillarIndex = 0 }
    schedule(guid, "berserk", 25 * 60 * 1000, function()
        onBerserk(creature, guid)
    end)
    armPhase1(creature, guid)
    schedule(guid, "taunt", 30000 + math.random(0, 30000), function()
        onTaunt(creature, guid)
    end)
end

-- C++ DamageTaken (pre-damage hook): lethal-from-anyone-but-self
-- -> health-1; the _dead outro path or the demon-form cancel
-- path. Then the 90%/65%/30% phase arms (strictly-below-pct on
-- current health, C++ HealthBelowPct — pre-damage value).
local function illidanDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = getIllidanState(guid)
    local health = creature:GetHealth()
    if health > 0 and damage >= health
            and (attacker == nil or attacker:GetGUID() ~= guid) then
        damage = health - 1
        if not state.dead then
            if state.isDemon then
                cancelTimers(guid)
                creature:CastSpell(creature, SPELL_DEMON_TRANSFORM_1,
                    false)
            else
                state.dead = true
                cancelTimers(guid)
                -- The parasitic-shadowfiend summon despawn arm has
                -- no summon bridge.
                creature:CastSpell(creature,
                    SPELL_REMOVE_PARASITIC_SHADOWFIEND, true)
                -- C++ DoAction(ACTION_START_OUTRO): triggered
                -- self-cast death 41218 (the NOT_SELECTABLE arm has
                -- no flag bridge), defeated Talk 4s, quiet suicide
                -- 3617 18s later. The maiev DoAction(ACTION_START_
                -- OUTRO) and summon-despawn arms have no bridges.
                creature:CastSpell(creature, SPELL_DEATH, true)
                schedule(guid, "defeatedText", 4000, function()
                    creature:Talk(SAY_ILLIDAN_DEFEATED)
                    schedule(guid, "quietSuicide", 18000, function()
                        creature:CastSpell(creature,
                            SPELL_QUIET_SUICIDE, true)
                    end)
                end)
            end
        end
        return false, damage
    end
    local healthPct = creature:GetHealthPct()
    if healthPct < 90 and state.phase < PHASE_MINIONS then
        state.phase = PHASE_MINIONS
        creature:Talk(SAY_ILLIDAN_MINION)
        -- The akama DoAction(ACTION_START_MINIONS) relay has no
        -- cross-creature bridge; the weave is armed locally.
        schedule(guid, "minionsWeave", 30000, function()
            onMinionsWeave(creature, guid)
        end)
    elseif healthPct < 65 and state.phase < PHASE_2 then
        state.phase = PHASE_2
        -- C++ DoAction(ACTION_START_PHASE_2): cancels GROUP_PHASE_1
        -- (the react/flag/emote/sound/gravity arms have no bridges).
        for _, key in ipairs({ "flameCrash", "drawSoul", "shear",
                "parasitic" }) do
            cancelTimer(guid, key)
        end
        schedule(guid, "fly", 1000, function()
            onFly(creature, guid)
        end)
    elseif healthPct < 30 and state.phase < PHASE_4 then
        state.phase = PHASE_4
        if state.isDemon then
            state.isDemon = false
            -- The root/interrupt arms have no bridges; the pending
            -- 72s cancel-demon timer is kept, C++-exact.
            cancelPhaseSchedule(guid)
            creature:CastSpell(creature, SPELL_DEMON_TRANSFORM_1,
                true)
            schedule(guid, "phase4delayed", 12000, function()
                startPhase4(creature, guid)
            end)
        else
            startPhase4(creature, guid)
        end
    end
end

-- C++ KilledUnit: Talk(SAY_ILLIDAN_KILL), TYPEID_PLAYER gate
-- (terestian convention).
local function illidanTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_ILLIDAN_KILL)
    end
end

local function illidanCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    illidanState[guid] = nil
end

RegisterCreatureEvent(ENTRY_ILLIDAN, 1, illidanEnterCombat)
RegisterCreatureEvent(ENTRY_ILLIDAN, 2, illidanCleanup)
RegisterCreatureEvent(ENTRY_ILLIDAN, 3, illidanTargetDied)
RegisterCreatureEvent(ENTRY_ILLIDAN, 4, illidanCleanup)
RegisterCreatureEvent(ENTRY_ILLIDAN, 9, illidanDamageTaken)
RegisterCreatureEvent(ENTRY_ILLIDAN, 23, illidanCleanup)

--
-- Akama (23089)
--

-- C++ DoAction(ACTION_START_ENCOUNTER) -> EVENT_HEALING_POTION:
-- health strictly below 20% -> non-triggered self-cast 40535,
-- re-arm 1s. Modeled on pull (the DoAction delivery has no
-- bridge).
local function onHealingPotion(creature, guid)
    if creature:GetHealthPct() < 20 then
        creature:CastSpell(creature, SPELL_HEALING_POTION, false)
    end
    schedule(guid, "healingPotion", 1000, function()
        onHealingPotion(creature, guid)
    end)
end

local function akamaEnterCombat(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "healingPotion", 1000, function()
        onHealingPotion(creature, guid)
    end)
end

-- C++ DamageTaken: lethal damage -> health-1 (C++-exact; no
-- self-exclusion in the C++).
local function akamaDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health > 0 and damage >= health then
        return false, health - 1
    end
end

local function akamaCleanup(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_AKAMA, 1, akamaEnterCombat)
RegisterCreatureEvent(ENTRY_AKAMA, 2, akamaCleanup)
RegisterCreatureEvent(ENTRY_AKAMA, 4, akamaCleanup)
RegisterCreatureEvent(ENTRY_AKAMA, 9, akamaDamageTaken)
RegisterCreatureEvent(ENTRY_AKAMA, 23, akamaCleanup)

--
-- Flame of Azzinoth (22997)
--

-- C++ ChargeTargetSelector: player, 2D distance > 25 yd from both
-- blade positions.
local function chargeTarget(creature)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        local dx1, dy1 = p:GetX() - BLADE_X1, p:GetY() - BLADE_Y1
        local dx2, dy2 = p:GetX() - BLADE_X2, p:GetY() - BLADE_Y2
        if (dx1 * dx1 + dy1 * dy1) > 25 * 25
                and (dx2 * dx2 + dy2 * dy2) > 25 * 25 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_FLAME_BLAST: non-triggered self-cast 40631 (C++
-- DoCastAOE); repeat 12s.
local function onFlameBlast(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_BLAST, false)
    schedule(guid, "flameBlast", 12000, function()
        onFlameBlast(creature, guid)
    end)
end

-- C++ EVENT_FLAME_CHARGE: non-triggered 42003 on the charge
-- target; repeat 5s with a target, 1s without (C++-exact).
local function onFlameCharge(creature, guid)
    local target = chargeTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHARGE, false)
        schedule(guid, "flameCharge", 5000, function()
            onFlameCharge(creature, guid)
        end)
    else
        schedule(guid, "flameCharge", 1000, function()
            onFlameCharge(creature, guid)
        end)
    end
end

-- C++ EVENT_ENGAGE (3s): the react/zone-combat arms have no
-- bridges; arms the charge and keeps the blast cycle.
local function onEngage(creature, guid)
    schedule(guid, "flameCharge", 5000, function()
        onFlameCharge(creature, guid)
    end)
end

local function flameEnterCombat(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "engage", 3000, function()
        onEngage(creature, guid)
    end)
    schedule(guid, "flameBlast", 11000, function()
        onFlameBlast(creature, guid)
    end)
end

-- C++ Reset: triggered self-cast 39856 (C++-exact; the boss-state
-- despawn check is blocked on the instance-script model, REACT_
-- PASSIVE has no bridge). The Reset timer arming is redundant
-- with the EnterCombat arming — modeled on combat only.
local function flameReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_FLAME_TEAR_OF_AZZINOTH, true)
end

local function flameCleanup(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FLAME_OF_AZZINOTH, 1, flameEnterCombat)
RegisterCreatureEvent(ENTRY_FLAME_OF_AZZINOTH, 2, flameCleanup)
RegisterCreatureEvent(ENTRY_FLAME_OF_AZZINOTH, 4, flameCleanup)
RegisterCreatureEvent(ENTRY_FLAME_OF_AZZINOTH, 23, flameReset)

--
-- Illidan DB target (23070)
--

-- C++ Reset: non-triggered self-cast 40017 (vaelastrasz
-- convention; the MovementInform aura-removal arm has no movement
-- bridge).
local function dbTargetReset(event, creature)
    creature:CastSpell(creature, SPELL_EYE_BLAST_TRIGGER, false)
end

RegisterCreatureEvent(ENTRY_DB_TARGET, 23, dbTargetReset)

--
-- Maiev Shadowsong (23197)
--

local maievState = {}

local function getMaievState(guid)
    local state = maievState[guid]
    if state == nil then
        state = { canDown = true }
        maievState[guid] = state
    end
    return state
end

-- C++ EVENT_CAGE_TRAP: non-triggered self-cast 40694 + Talk;
-- repeat 30s. The illidan->CastSpell(CAGED_TRAP_TELEPORT 40693)
-- arm has no cross-creature bridge — skipped.
local function onCageTrap(creature, guid)
    creature:CastSpell(creature, SPELL_CAGE_TRAP_SUMMON, false)
    creature:Talk(SAY_MAIEV_TRAP)
    schedule(guid, "cageTrap", 30000, function()
        onCageTrap(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_STRIKE: non-triggered DoCastVictim; repeat 50s.
local function onShadowStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_STRIKE, false)
    end
    schedule(guid, "shadowStrike", 50000, function()
        onShadowStrike(creature, guid)
    end)
end

-- C++ EVENT_THROW_DAGGER: victim beyond melee range ->
-- non-triggered DoCastVictim, repeat 5s; else repeat 1s (melee
-- range approximated at 5 yd — documented).
local function onThrowDagger(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:GetDistance(victim) > 5 then
        creature:CastSpell(victim, SPELL_THROW_DAGGER, false)
        schedule(guid, "throwDagger", 5000, function()
            onThrowDagger(creature, guid)
        end)
    else
        schedule(guid, "throwDagger", 1000, function()
            onThrowDagger(creature, guid)
        end)
    end
end

-- C++ EVENT_TAUNT (from JustAppeared): Talk; repeat {30s,60s}.
local function onMaievTaunt(creature, guid)
    creature:Talk(SAY_MAIEV_TAUNT)
    schedule(guid, "maievTaunt", 30000 + math.random(0, 30000),
        function()
            onMaievTaunt(creature, guid)
        end)
end

-- C++ JustEngagedWith: the phase/combat arms; the appear sequence
-- (summon-driven) is unmodeled, so the combat arms start on pull
-- (the taunt arm is relocated here from JustAppeared —
-- documented).
local function maievEnterCombat(event, creature)
    local guid = creature:GetGUID()
    maievState[guid] = { canDown = true }
    schedule(guid, "cageTrap", 30000, function()
        onCageTrap(creature, guid)
    end)
    schedule(guid, "shadowStrike", 50000, function()
        onShadowStrike(creature, guid)
    end)
    schedule(guid, "throwDagger", 1000, function()
        onThrowDagger(creature, guid)
    end)
    schedule(guid, "maievTaunt", 20000 + math.random(0, 40000),
        function()
            onMaievTaunt(creature, guid)
        end)
end

-- C++ DamageTaken: lethal while _canDown -> health-1, triggered
-- self-cast 40409, Talk(SAY_MAIEV_SHADOWSONG_DOWN), _canDown=false
-- (the ACTION_MAIEV_DOWN_FADE re-arm lives in the unmodeled
-- maiev-down AuraScript — standing AuraScript gap).
local function maievDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = getMaievState(guid)
    local health = creature:GetHealth()
    if health > 0 and damage >= health and state.canDown then
        state.canDown = false
        creature:CastSpell(creature, SPELL_MAIEV_DOWN, true)
        creature:Talk(SAY_MAIEV_DOWN)
        return false, health - 1
    end
end

local function maievCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    maievState[guid] = nil
end

RegisterCreatureEvent(ENTRY_MAIEV, 1, maievEnterCombat)
RegisterCreatureEvent(ENTRY_MAIEV, 2, maievCleanup)
RegisterCreatureEvent(ENTRY_MAIEV, 4, maievCleanup)
RegisterCreatureEvent(ENTRY_MAIEV, 9, maievDamageTaken)
RegisterCreatureEvent(ENTRY_MAIEV, 23, maievCleanup)

--
-- Blade of Azzinoth (22996)
--

-- C++ Reset: triggered self-cast 40031 (C++-exact; the spawn
-- sound, the tear-of-azzinoth summon chain and the boss-state
-- despawn check have no sound/summon/instance bridges).
local function bladeReset(event, creature)
    creature:CastSpell(creature, SPELL_BIRTH, true)
end

RegisterCreatureEvent(ENTRY_BLADE_OF_AZZINOTH, 23, bladeReset)

--
-- Illidan generic fire (23069 demon fire / 23259 blaze /
-- 23336 flame crash)
--

-- C++ Reset entry switch, all triggered self-casts (C++-exact).
local function genericFireReset(event, creature)
    local entry = creature:GetEntry()
    if entry == ENTRY_DEMON_FIRE then
        creature:CastSpell(creature, SPELL_DEMON_FIRE, true)
    elseif entry == ENTRY_BLAZE then
        creature:CastSpell(creature, SPELL_BLAZE, true)
        creature:CastSpell(creature, SPELL_BIRTH, true)
    elseif entry == ENTRY_FLAME_CRASH then
        creature:CastSpell(creature, SPELL_FLAME_CRASH_GROUND, true)
    end
end

RegisterCreatureEvent(ENTRY_DEMON_FIRE, 23, genericFireReset)
RegisterCreatureEvent(ENTRY_BLAZE, 23, genericFireReset)
RegisterCreatureEvent(ENTRY_FLAME_CRASH, 23, genericFireReset)
