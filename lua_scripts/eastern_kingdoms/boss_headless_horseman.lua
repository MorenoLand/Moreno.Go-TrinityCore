-- Boss Headless Horseman (Scarlet Monastery)
-- Lua port of src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_headless_horseman.cpp
-- (struct boss_headless_horseman : public ScriptedAI, registered by
-- AddSC_boss_headless_horseman in eastern_kingdoms_script_loader.cpp
-- (line 279) via RegisterScarletMonasteryCreatureAI ->
-- RegisterCreatureAIWithFactory (scarlet_monastery.h:100)).
-- Entry (verifiable from the C++ sources): NPC_HEADLESS_HORSEMAN =
-- 23682 in the SMCreatureIds enum in ScarletMonastery/
-- scarlet_monastery.h (lines 69-75, same-block header enum); the same
-- enum also names NPC_HEADLESS_HORSEMAN_HEAD = 23775 /
-- NPC_PULSING_PUMPKIN = 23694 / NPC_PUMPKIN_FIEND = 23545 /
-- NPC_FLAME_BUNNY = 23686 / NPC_EARTH_BUNNY = 23758 /
-- NPC_SIR_THOMAS = 23904, and SMGameObjectIds names
-- GO_PUMPKIN_SHRINE = 186267 / GO_LOOSELY_TURNED_SOIL = 186314.
-- Whole-server-tree grep confirms boss_headless_horseman.cpp as the
-- only source of "boss_headless_horseman" (struct line 340,
-- AddSC_ line 1049).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Verifiable numbers (file's own enums): SPELL_HEADLESS_HORSEMAN_CLEAVE =
-- 42587 / SPELL_HEADLESS_HORSEMAN_BURNING_COSMETIC = 42971
-- (HeadlessHorsemanSpells); SAY_HORSEMAN_ENTRANCE = 0 / SAY_REJOINED = 1 /
-- SAY_CONFLAGRATION = 2 / SAY_SPROUTING_PUMPKINS = 3 / SAY_DEATH = 4 /
-- SAY_KILL_PLAYER = 5 (HeadlessHorsemanSays, Boss leg); EVENT_HORSEMAN_CLEAVE
-- = 1 (HeadlessEvents); Cleave 13s init -> Repeat(6s, 12s) DoCastVictim;
-- phase-2 cleave re-arm 16s, phase-3 re-arm 9s.
-- Ported arms:
-- - JustEngagedWith's EVENT_HORSEMAN_CLEAVE -> CreateLuaEvent timer armed
--   on OnEnterCombat (1), cancelled on 2/4/23 (maiden convention), with
--   DoCastVictim -> GetVictim + CastSpell (mr_smite convention),
--   C++-exact 13s init / Repeat(6s, 12s).
-- - KilledUnit's Talk(SAY_KILL_PLAYER) with the TYPEID_PLAYER gate ->
--   OnTargetDied (3) with a victim:GetObjectType() == "Player" gate (illidan convention).
-- - JustDied's Talk(SAY_DEATH) + DoCastSelf(SPELL_HEADLESS_HORSEMAN_
--   BURNING_COSMETIC) -> OnDied (4) (maiden precedent: cancel timers,
--   then the death arm); the cosmetic cast is packet-visual like all
--   creature self-casts on this surface.
-- Deviations from C++: the 42410 reposition leg has no bridge
-- (NearTeleportTo zero hits engine-wide) and is skipped — the head's
-- MoveRandom(30) in HandleStartPhase repositions it anyway. The 42603
-- HP check runs on a 1s poll (live path) plus the RegisterAuraEvent
-- below (DBC path); the DBC period for 42603 is unverified in-repo.
-- JustDied's _instance->SetData(DATA_HORSEMAN_EVENT_STATE, DONE) leg
-- is blocked on the instance-script model; the SummonCreature
-- (NPC_SIR_THOMAS) leg is PORTED (creature:SummonCreature bridge,
-- lua_creature_events.go:4161 — the stale "no summon bridge" claim
-- retracted; Eluna type 7 = TEMPSUMMON_MANUAL_DESPAWN, the C++
-- default); the LFG FinishDungeon leg has no LFG bridge.
-- JustEngagedWith's setActive leg has no bridge; its
-- SetCombatPulseDelay(5) arm is PORTED (creature:SetCombatPulseDelay
-- bridge, lua_creature_events.go; event 1, C++ arm order). Its
-- EVENT_RANDOM_LAUGH arm is ported as the
-- onLaugh timer via creature:PlayDirectSound (SMSG_PLAY_SOUND); its
-- head->AI()->DoZoneInCombat() leg hangs off the instance-script model.
-- - UpdateAI CASTING gates PORTED for the body arms above
--   (boss_headless_horseman.cpp:547/:606): cleave + laugh defer 500ms
--   while creature:IsCasting(), no re-arm consumed. The head's own
--   UpdateAI gates (boss_headless_horseman.cpp:297/:329) have no
--   observable Lua effect: its only timer is the post-death
--   raintreats one-shot, and a dead creature has no cast in flight.
--   Pumpkin/bunny/sir-thomas arms carry no UpdateAI override in C++
--   (ScriptedAI::UpdateAI is the empty base), so nothing gates there.
-- Unmodeled (documented-only — no bridges on the Lua surface):
-- - DamageTaken's phase-transition latch + StartPhase + the whole
--   head-phase machine: PORTED 2026-11-14 (round109) — ev9 damage
--   rewrite, Lua-side phase variable, DoAction/ev38 cross-AI dispatch
--   with FindNearestCreature head/boss lookups, MoveTo + ev28 arrival,
--   conflagrate + pumpkin phase timers. See the machine code below.
-- - DoAction's ACTION_HORSEMAN_EVENT_START (flight-path intro via
--   MoveSmoothPath + yell-timer/laugh auras) stays out (no
--   MoveSmoothPath bridge); REQUEST_BODY / RETURN_TO_BODY / IS_DEAD
--   are PORTED on ev38 (below).
-- - EVENT_CONFLAGRATE (phase 2) and EVENT_SUMMON_PUMPKIN (phase 3):
--   PORTED 2026-11-14 (round109) as phase timers (conflagrateTick /
--   pumpkinTick); the 52236 pumpkin-spawn machinery itself is the
--   aura's DBC effect, and the Talk(SAY_SPROUTING_PUMPKINS) leg rides
--   the RegisterAuraEvent below.
-- - Reset/JustReachedHome/InitializeAI legs: the SetDisableGravity/
--   SetHover legs port on event 5 (InitializeAI twin, both bridged —
--   stale "no bridge" retracted); the POINT_HORSEMAN_FINISH_PATH
--   disable legs ride the unported intro flight-path machine (DoAction
--   ACTION_HORSEMAN_EVENT_START / MoveSmoothPath, documented above).
-- - Reset/JustReachedHome legs: _summons.DespawnAll is PORTED
--   2026-11-14 (round109) — the tracked horsemanSummons table (now
--   storing the summoned objects, felblood phoenix precedent) drains
--   in onCombatEnd, alive-gated (the death path returns early, moroes
--   precedent); SetReactState (SetReactState IS bridged — frozen_core
--   precedent, lua_creature_events.go:524; the JustReachedHome
--   REACT_AGGRESSIVE leg is PORTED in onReachHome below),
--   SetImmuneToPC (twin bridged — SetFlag/RemoveFlag(59, 0x100); the Reset
--   clear stays out on its _introDone gate: _introDone is set only by the
--   unported POINT_HORSEMAN_FINISH_PATH flight-path leg, so no clear can
--   fire in the Lua model). HandleInitialSetup's
--   DoCastSelf(42413/43877) self-casts were
--   "stranded halves" (rageclaw/phoenix_tk precedent) only before the ev5
--   spawn bridge landed; they now port on event 5 below, which fires at the
--   exact C++ JustAppeared moments (fireCreatureSpawned, initial + respawn).
-- - npc_headless_horseman_head (23775): REGISTERED — JustDied's
--   EVENT_RAIN_OF_TREATS 11s -> DoCastSelf(43344) ports cleanly as a
--   single-arm registration. The head-phase machine (DoAction
--   START_HEAD_PHASE / HP_67 / HP_34, the 42603 HP check, STOP +
--   return, head-is-dead) is PORTED 2026-11-14 (round109) on ev38 +
--   the 42603 aura event + a 1s HP poll (see below); the head's
--   _instance->GetCreature(DATA_HEADLESS_HORSEMAN) lookups ride
--   FindNearestCreature. Still out: SetGUID(ACTION_HEAD_PLAYER_TEXT)
--   (no SetGUID bridge; the GO gossip senders are unported too) and
--   the JustReachedHome motion Clear (StopMoving twin is live but the
--   head carries no Lua motion to clear).
-- - spell_headless_horseman_summoning_rhyme_aura (42879, tick 189
--   audit): the PeriodicTick switch is keyed on
--   aurEff->GetTickNumber() (1/3/5/8) — PORTED below (tick 191,
--   RegisterAuraEvent, aura:GetTickNumber bridge); the arms are
--   aura:GetCaster() + caster:Talk(SAY_*, player) (mandokir whisper
--   precedent), player:CastSpell(player, 42910/42909, true),
--   aura:Remove().
-- - npc_pulsing_pumpkin (23694): PORTED below. Reset's own 42281
--   self-cast IS the DoAction trigger in C++ (the 42281 SpellScript
--   hits self -> ACTION_PUMPKIN_SPROUTING_FINISHED), so the sprout
--   chain runs as timers from Reset: 1s -> RemoveAura 42280 +
--   gravity/hover off + triggered 42285 + UpdateEntry(23545) +
--   SetImmuneToNPC + REACT_AGGRESSIVE + DoZoneInCombat; 11s ->
--   DoCastVictim 42514 + Repeat 10s. Deltas: timers are keyed (a
--   second Reset re-arms rather than duplicating TaskScheduler
--   tasks); Lua timers are not victim-gated (C++ ticks only in
--   combat — the squash cast is victim-guarded, so an unengaged
--   pumpkin casts nothing); timers cancel on death only (C++
--   tasks pend across evade, same effect). Bearer is latent:
--   pumpkins spawn only via the unported 52236 aura summon.
-- - npc_flame_bunny (23686): PORTED below. Ctor display Modelid2 on
--   event 5 (hodir precedent) + Reset's scheduler as Lua timers: 1s
--   -> self-cast 43184; 3s -> self-cast 43148, re-armed 2.2s twice
--   (3 casts total, C++ repeat-counter < 2). C++ UpdateAI ticks the
--   scheduler ungated, so Lua timers are faithful here; timers
--   cancel on death only. Bearer is latent (no C++ summoner in-tree
--   outside the encounter machinery).
-- - npc_sir_thomas (23904): Reset self-casts + a SpellHit(42818) handler
--   — SpellHit handlers not modeled (draenei_survivor precedent).
--   Documented-only.
-- - go_loosely_turned_soil (186314) / go_headless_horseman_pumpkin
--   (186267): OnGossipHello/OnQuestReward/OnGossipSelect all hang off
--   _instance->GetData/SetData (DATA_HORSEMAN_EVENT_STATE/DATA_START_
--   HORSEMAN_EVENT — instance-script model blocked) plus the gossip and
--   quest-reward arms, which have no bridges on this surface.
--   Documented-only.

local SPELL_HORSEMAN_CLEAVE = 42587
local SPELL_BURNING_COSMETIC = 42971
local SPELL_RAIN_OF_TREATS = 43344
local SPELL_HEAD_STUN = 42408
local GO_PUMPKIN_SHRINE = 186267
local GO_LOOSELY_TURNED_SOIL = 186314
-- UNIT_FIELD_FLAGS (UnitDefines.h) / UNIT_FLAG_NOT_SELECTABLE — the
-- EVENT_RAIN_OF_TREATS SetFlag leg (bridged creature:SetFlag).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local SPELL_HEAD_VISUAL = 42413
local SPELL_ONKILL_PROC = 43877

local SAY_DEATH = 4
local SAY_KILL_PLAYER = 5
local SAY_SPROUTING_PUMPKINS = 3 -- boss_headless_horseman.cpp:53
local SOUNDID_MANIACAL_LAUGH = 11975 -- boss_headless_horseman.cpp:150
local SPELL_MANIACAL_LAUGH_AURA = 43893 -- boss_headless_horseman.cpp:814
local SPELL_SUMMON_PUMPKIN_BURST_DELAY = 52236 -- boss_headless_horseman.cpp:934
local SPELL_SUMMONING_RHYME_AURA = 42879 -- boss_headless_horseman.cpp:90
local SPELL_SUMMONING_RHYME_SHAKE_SMALL = 42910 -- boss_headless_horseman.cpp:92
local SPELL_SUMMONING_RHYME_SHAKE_MEDIUM = 42909 -- boss_headless_horseman.cpp:91
local SAY_PLAYER_RISE = 2 -- boss_headless_horseman.cpp:44
local SAY_PLAYER_TIME = 3 -- boss_headless_horseman.cpp:45
local SAY_PLAYER_DEATH = 4 -- boss_headless_horseman.cpp:46
local SAY_PLAYER_DEMISE = 5 -- boss_headless_horseman.cpp:47

local ENTRY_HEADLESS_HORSEMAN = 23682
local ENTRY_HORSEMAN_HEAD = 23775
-- boss_headless_horseman.cpp HeadlessActions (HeadlessActions enum):
-- the head's DoAction arms ride ev38 (below); the boss's ride ev38 too.
local ACTION_HEAD_START_HEAD_PHASE = 1
local ACTION_HEAD_HP_67 = 2
local ACTION_HEAD_HP_34 = 3
local ACTION_HEAD_IS_DEAD = 4
local ACTION_HORSEMAN_REQUEST_BODY = 5
local ACTION_HEAD_RETURN_TO_BODY = 6
-- EventMap phases (HeadlessHorsemanMisc): the boss phase machine runs
-- on a Lua-side phase variable (horsemanPhase, below); the head's own
-- _phase runs on headPhase.
local PHASE_1 = 1
local PHASE_2 = 2
local PHASE_3 = 3
local PHASE_LAST = 4
local HEAD_PHASE_DEAD = 5
local POINT_HEAD = 2
local SAY_REJOINED = 1
local SAY_CONFLAGRATION = 2
local SAY_HEAD_REQUEST_BODY = 1
local SPELL_BODY_REGEN_IMMUNE = 42556
local SPELL_BODY_REGEN_CONFUSE = 43105
local SPELL_BODY_REGEN_TRANSFORM = 42403
local SPELL_HEAD_PERIODIC = 42603
local SPELL_HEAL_BODY = 43306
local SPELL_HEAD_PAUSE = 42504
local SPELL_SPEAKS = 43129
local SPELL_BODY_STAGE_1 = 42547
local SPELL_BODY_STAGE_2 = 42548
local SPELL_BODY_STAGE_3 = 42549
local SPELL_HORSEMANS_WHIRLWIND = 43116
local SPELL_BODY_DEATH = 42429
local SPELL_CONFLAGRATION_SOUND = 48149
local SPELL_CONFLAGRATION_SOUND_THROTTLE = 48148
local SPELL_CONFLAGRATION = 42380
local ENTRY_SIR_THOMAS = 23904 -- NPC_SIR_THOMAS, scarlet_monastery.h:75
local SPELL_WISP_INVIS = 42823 -- boss_headless_horseman.cpp:106
local SPELL_WISP_FLIGHT_PORT = 42818 -- boss_headless_horseman.cpp:108
local SPELL_HEADLESS_HORSEMAN_C_GHOST_VISUAL = 42575 -- boss_headless_horseman.cpp:109
local SPELL_WISP_FLIGHT_MISSILE = 42821 -- boss_headless_horseman.cpp:107
local UNIT_NPC_FLAGS_FIELD = 82 -- aeranas precedent
local UNIT_NPC_FLAG_QUESTGIVER = 2

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

-- Cancel one keyed timer (the HP-check poll cancels itself when its
-- arm fires — the C++ 42603 aura is removed by the same arm).
local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ boss_headless_horseman::UpdateAI pre-loop + in-loop CASTING
-- gates PORTED (boss_headless_horseman.cpp:547/:606): the arm defers
-- 500ms while creature:IsCasting(), no re-arm consumed.
local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            onCleave(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HORSEMAN_CLEAVE)
    end
    schedule(guid, "cleave", math.random(6000, 12000), function() onCleave(creature, guid) end)
end

local LAUGH_SOUNDS = { 11975, 11965, 11976 }

-- C++ EVENT_RANDOM_LAUGH: DoPlaySoundToSet(me, random laugh) =
-- creature:PlayDirectSound (SMSG_PLAY_SOUND); initial {30s,60s},
-- re-arm {30s,60s} (C++-exact).
-- CASTING gate PORTED as above: the sound event stays pending
-- while a cast is in flight (C++ pre-loop return skips the whole
-- iteration), so it defers 500ms instead of playing over a cast.
local function onLaugh(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "laugh", 500, function()
            onLaugh(creature, guid)
        end)
        return
    end
    creature:PlayDirectSound(LAUGH_SOUNDS[math.random(3)])
    schedule(guid, "laugh", math.random(30000, 60000), function() onLaugh(creature, guid) end)
end

-- The head-phase machine (boss_headless_horseman.cpp:459-502, 540-603,
-- PORTED 2026-11-14 round109): the boss phase lives in horsemanPhase
-- (EventMap phase twin; C++ SetPhase has no Lua bridge). The 42410
-- reposition leg has no bridge (NearTeleportTo zero hits engine-wide)
-- and is SKIPPED as a documented deviation — HandleStartPhase's
-- MoveRandom(30) repositions the head anyway.
local horsemanPhase = {}

-- Forward-declared here (tharon_ja / gal_darah idiom): the
-- horsemanSummons table itself is declared with the JustSummoned
-- handler further below, but drainHorsemanSummons (called from
-- onCombatEnd) reads it — without this forward declaration the
-- reference would bind the nil global (malown time-bomb; the
-- use-before-def scan flags exactly this).
local horsemanSummons

-- Forward declarations (tharon_ja / gal_darah idiom): startPhase and
-- startNextPhaseDelayed call these before their definitions below;
-- plain assignments, never re-local-function (malown time-bomb).
local startNextHeadlessPhase, conflagrateTick, pumpkinTick

local function drainHorsemanSummons()
    for guid, summoned in pairs(horsemanSummons) do
        if summoned then
            summoned:DespawnOrUnsummon()
        end
        horsemanSummons[guid] = nil
    end
end

-- C++ StartPhase (boss_headless_horseman.cpp:473-502): immune self-cast
-- (triggered), SetPhase, REACT_PASSIVE, AttackStop, whirlwind removal,
-- SetWalk(false), stage-aura removal per phase, then the 1ms
-- EVENT_START_NEXT_HEADLESS_PHASE. The phase-1 cleave is phase-pinned
-- in C++ (never fires again); the Lua twin cancels it here and the
-- delayed re-engage re-arms it phase-pinned (16s / 9s).
local function startPhase(creature, guid, nextPhase)
    creature:CastSpell(creature, SPELL_BODY_REGEN_IMMUNE, true)
    horsemanPhase[guid] = nextPhase
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:AttackStop()
    creature:RemoveAura(SPELL_HORSEMANS_WHIRLWIND)
    creature:SetWalk(false)
    if nextPhase == PHASE_2 then
        creature:RemoveAura(SPELL_BODY_STAGE_1)
    elseif nextPhase == PHASE_3 then
        creature:RemoveAura(SPELL_BODY_STAGE_2)
        cancelKey(guid, "conflagrate")
    elseif nextPhase == PHASE_LAST then
        creature:RemoveAura(SPELL_BODY_STAGE_3)
        creature:RemoveAura(SPELL_SUMMON_PUMPKIN_BURST_DELAY)
        cancelKey(guid, "pumpkin")
    end
    cancelKey(guid, "cleave")
    schedule(guid, "start_head_phase", 1, function()
        startNextHeadlessPhase(creature, guid)
    end)
end

-- C++ EVENT_START_NEXT_HEADLESS_PHASE (boss_headless_horseman.cpp:
-- 560-569): DoCastAOE(42410) skipped (no bridge, see above);
-- DoCast(head, 42399, triggered) collapses to head:DoAction(1) (the
-- 42399 SpellScript is a pure DoAction dispatcher); confuse +
-- transform self-casts (triggered); head-visual removal.
startNextHeadlessPhase = function(creature, guid)
    local head = creature:FindNearestCreature(ENTRY_HORSEMAN_HEAD, 200)
    if head ~= nil then
        head:DoAction(ACTION_HEAD_START_HEAD_PHASE)
    end
    creature:CastSpell(creature, SPELL_BODY_REGEN_CONFUSE, true)
    creature:RemoveAura(SPELL_HEAD_VISUAL)
    creature:CastSpell(creature, SPELL_BODY_REGEN_TRANSFORM, true)
end

-- C++ EVENT_START_NEXT_PHASE_DELAYED (boss_headless_horseman.cpp:
-- 571-585): re-engage, head-visual self-cast, phase stage aura +
-- phase timers (conflagrate 5s / cleave 16s in phase 2; pumpkin 1s /
-- cleave 9s in phase 3; LAST re-engages with no phase timers), then
-- AttackStart on the max-threat target (GetAITargets is threat-sorted).
local function startNextPhaseDelayed(creature, guid)
    creature:SetReactState(2) -- REACT_AGGRESSIVE
    creature:CastSpell(creature, SPELL_HEAD_VISUAL)
    local phase = horsemanPhase[guid] or PHASE_1
    if phase == PHASE_2 then
        creature:CastSpell(creature, SPELL_BODY_STAGE_2)
        schedule(guid, "conflagrate", 5000, function()
            conflagrateTick(creature, guid)
        end)
        schedule(guid, "cleave", 16000, function() onCleave(creature, guid) end)
    elseif phase == PHASE_3 then
        creature:CastSpell(creature, SPELL_BODY_STAGE_3)
        schedule(guid, "pumpkin", 1000, function()
            pumpkinTick(creature, guid)
        end)
        schedule(guid, "cleave", 9000, function() onCleave(creature, guid) end)
    end
    local targets = creature:GetAITargets()
    if targets ~= nil and targets[1] ~= nil then
        creature:AttackStart(targets[1])
    end
end

-- C++ EVENT_CONFLAGRATE (boss_headless_horseman.cpp:586-595): random
-- player within 30yd; sound self-casts (triggered) + DoCast(target,
-- 42380) + Talk(SAY_CONFLAGRATION); Repeat(18s, 21s) even with no
-- target. CASTING gate PORTED as the 500ms defer (file convention).
conflagrateTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "conflagrate", 500, function()
            conflagrateTick(creature, guid)
        end)
        return
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    if targets ~= nil then
        for _, target in ipairs(targets) do
            if target:GetObjectType() == "Player" then
                local dx = target:GetX() - creature:GetX()
                local dy = target:GetY() - creature:GetY()
                local dz = target:GetZ() - creature:GetZ()
                if dx * dx + dy * dy + dz * dz <= 900 then
                    candidates[#candidates + 1] = target
                end
            end
        end
    end
    if #candidates > 0 then
        local target = candidates[math.random(#candidates)]
        creature:CastSpell(creature, SPELL_CONFLAGRATION_SOUND, true)
        creature:CastSpell(creature, SPELL_CONFLAGRATION_SOUND_THROTTLE, true)
        creature:CastSpell(target, SPELL_CONFLAGRATION)
        creature:Talk(SAY_CONFLAGRATION)
    end
    schedule(guid, "conflagrate", math.random(18000, 21000), function()
        conflagrateTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_PUMPKIN (boss_headless_horseman.cpp:596-599):
-- DoCastSelf(52236), Repeat(30s). The 52236 aura itself is unseeded,
-- so the cast is a no-op until a real Spell.dbc ships it; the Talk
-- leg rides the RegisterAuraEvent below when it applies.
pumpkinTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "pumpkin", 500, function()
            pumpkinTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_PUMPKIN_BURST_DELAY)
    schedule(guid, "pumpkin", 30000, function()
        pumpkinTick(creature, guid)
    end)
end

-- C++ boss_headless_horseman::DamageTaken (boss_headless_horseman.cpp:
-- 459-471): lethal non-self hit -> damage = 0 + StartPhase(next);
-- PHASE_LAST takes full damage (no arm). ev9 rewrite convention:
-- the second return rewrites damage (fireCreatureDamageTaken,
-- lua_creature_events.go:7684). The attacker-vs-self gate compares
-- GUIDs; a missing GetGUIDLow (non-unit attacker) falls through as
-- non-self, matching C++ in practice (no self-damage source exists
-- in this fight).
local function onDamageTaken(_, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        local guid = creature:GetGUIDLow()
        local attackerGuid = nil
        if attacker ~= nil and attacker.GetGUIDLow ~= nil then
            attackerGuid = attacker:GetGUIDLow()
        end
        if attackerGuid ~= guid then
            local phase = horsemanPhase[guid] or PHASE_1
            if phase == PHASE_1 then
                startPhase(creature, guid, PHASE_2)
            elseif phase == PHASE_2 then
                startPhase(creature, guid, PHASE_3)
            elseif phase == PHASE_3 then
                startPhase(creature, guid, PHASE_LAST)
            end
            return true, 0
        end
    end
end

-- C++ boss_headless_horseman::DoAction (boss_headless_horseman.cpp:
-- 430-469) on ev38 (creature:DoAction bridge, lua_creature_events.go:
-- 845). ACTION_HORSEMAN_EVENT_START (intro flight path) stays out
-- (no MoveSmoothPath bridge).
local function onDoAction(_, creature, action)
    if action == ACTION_HORSEMAN_REQUEST_BODY then
        -- :447-457: remove confuse, clear motion, whirlwind self-cast
        -- (triggered), then walk to the head (MoveTo bridges MovePoint,
        -- lua_creature_events.go:5424; arrival via ev28 below).
        creature:RemoveAura(SPELL_BODY_REGEN_CONFUSE)
        creature:StopMoving()
        creature:CastSpell(creature, SPELL_HORSEMANS_WHIRLWIND, true)
        local head = creature:FindNearestCreature(ENTRY_HORSEMAN_HEAD, 200)
        if head ~= nil then
            creature:SetWalk(true)
            creature:MoveTo(POINT_HEAD, head:GetX(), head:GetY(), head:GetZ())
        end
    elseif action == ACTION_HEAD_RETURN_TO_BODY then
        -- :458-464: remove confuse/immune/transform, Talk(SAY_REJOINED),
        -- re-engage delayed 3s.
        creature:RemoveAura(SPELL_BODY_REGEN_CONFUSE)
        creature:RemoveAura(SPELL_BODY_REGEN_IMMUNE)
        creature:RemoveAura(SPELL_BODY_REGEN_TRANSFORM)
        creature:Talk(SAY_REJOINED)
        local guid = creature:GetGUIDLow()
        schedule(guid, "next_phase_delayed", 3000, function()
            startNextPhaseDelayed(creature, guid)
        end)
    elseif action == ACTION_HEAD_IS_DEAD then
        -- :465-469: remove immune, self-cast body-death (triggered).
        creature:RemoveAura(SPELL_BODY_REGEN_IMMUNE)
        creature:CastSpell(creature, SPELL_BODY_DEATH, true)
    end
end

-- C++ boss_headless_horseman::MovementInform (boss_headless_horseman.
-- cpp:515-534) on ev28: POINT_HEAD arrival -> walk off, whirlwind
-- removal, confuse self-cast (triggered). The intro finish-path leg
-- stays out (MoveSmoothPath). MoveTo is the only motion the boss uses
-- in this file, so the pointId gate alone is exact.
local function onMovementInform(_, creature, moveType, pointId)
    if pointId == POINT_HEAD then
        creature:SetWalk(false)
        creature:RemoveAura(SPELL_HORSEMANS_WHIRLWIND)
        creature:CastSpell(creature, SPELL_BODY_REGEN_CONFUSE, true)
    end
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- boss_headless_horseman.cpp:382 (arm 1 of JustEngagedWith; setActive leg has no bridge)
    local guid = creature:GetGUIDLow()
    schedule(guid, "cleave", 13000, function() onCleave(creature, guid) end)
    schedule(guid, "laugh", math.random(30000, 60000), function() onLaugh(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() then
        -- C++ Reset (boss_headless_horseman.cpp:378-388): phase 1 +
        -- _summons.DespawnAll + SetWalk(false). Ev2/ev23 both route
        -- through Reset on evade; the death path returns early
        -- (moroes precedent), so a dead boss keeps its summons.
        horsemanPhase[creature:GetGUIDLow()] = PHASE_1
        drainHorsemanSummons()
        creature:SetWalk(false)
    end
end

local function onTargetDied(_, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:CastSpell(creature, SPELL_BURNING_COSMETIC)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (boss_headless_horseman.cpp:442-456) arm order:
    -- DoCastSelf(BURNING_COSMETIC) -> SetData (instance, blocked) ->
    -- Talk(SAY_DEATH) -> SummonCreature(NPC_SIR_THOMAS, me->GetPosition(),
    -- TEMPSUMMON_MANUAL_DESPAWN Eluna type 7, 0ms) -> LFG (blocked).
    creature:SummonCreature(ENTRY_SIR_THOMAS, creature:GetX(), creature:GetY(),
        creature:GetZ(), creature:GetO(), 7, 0)
end

-- C++ boss_headless_horseman::JustAppeared -> HandleInitialSetup
-- (boss_headless_horseman.cpp:357): two TRIGGERED self-casts.
local function onSpawn(_, creature)
    -- C++ boss_headless_horseman::InitializeAI
    -- (boss_headless_horseman.cpp:344-349): me->SetDisableGravity(true),
    -- me->SetHover(true), then ScriptedAI::InitializeAI() ->
    -- HandleInitialSetup (the two self-casts below + SetImmuneToPC +
    -- REACT_PASSIVE, the latter two documented-out in the header).
    creature:SetDisableGravity(true)
    creature:SetHover(true)
    creature:CastSpell(creature, SPELL_HEAD_VISUAL, true)
    creature:CastSpell(creature, SPELL_ONKILL_PROC, true)
end

-- C++ boss_headless_horseman::JustReachedHome
-- (boss_headless_horseman.cpp:373-378): HandleInitialSetup re-cast +
-- SetReactState(REACT_AGGRESSIVE 2); ScriptedAI::JustReachedHome is the
-- base no-op. Fires after onCombatEnd (ev2/ev23) cancelled timers, so the
-- re-cast lands on the clean post-evade state exactly like C++.
local function onReachHome(_, creature)
    creature:CastSpell(creature, SPELL_HEAD_VISUAL, true)
    creature:CastSpell(creature, SPELL_ONKILL_PROC, true)
    creature:SetReactState(2)
end

-- C++ boss_headless_horseman::JustSummoned (boss_headless_horseman.cpp:391-395):
-- _summons.Summon(summon) + DoZoneInCombat(summon). The Summon leg is
-- the C++ SummonList (ScriptedCreature.cpp:43-45: GUID appended, no
-- spawn side effect); horsemanSummons is its Lua twin, keyed by the
-- summoned creature's GUIDLow. DoZoneInCombat engages the summon in
-- zone combat (drakkari precedent). Live summoners in this file: the
-- boss's own onDied SummonCreature(NPC_SIR_THOMAS 23904) below, and
-- aura-spawned pulsing pumpkins when the summon effect stamps the
-- horseman as owner (both arrive here via fireCreatureSummoned,
-- lua_creature_events.go:7897, args (event, creature, summoned)).
-- SummonedCreatureDies (:397-400) is _summons.Despawn(summon):
-- ScriptedCreature.cpp:47-50 removes the GUID from the list ONLY — no
-- DespawnOrUnsummon — so the ev21 handler drops the GUID and despawns
-- nothing. Dispatched live on the summoner via the summon's OwnerGUID
-- (lua_creature_events.go:7744-7771, args (event, summoner, summon,
-- killer)). Reset's DespawnAll leg drains the table in onCombatEnd
-- (above); the table exists so a future Reset port has the C++-exact
-- list to drain.
horsemanSummons = {}

local function onJustSummoned(_, creature, summoned)
    -- The table now stores the summoned object (felblood phoenix
    -- precedent) so Reset's DespawnAll leg can drain it; ev21 still
    -- drops the GUID on death.
    horsemanSummons[summoned:GetGUIDLow()] = summoned
    summoned:DoZoneInCombat()
end

local function onSummonedCreatureDies(_, creature, summon)
    horsemanSummons[summon:GetGUIDLow()] = nil
end

RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 4, onDied)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 5, onSpawn)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 23, onCombatEnd)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 24, onReachHome)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 38, onDoAction)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_HEADLESS_HORSEMAN, 21, onSummonedCreatureDies)

-- npc_headless_horseman_head (23775, scarlet_monastery.h:70):
-- JustDied schedules EVENT_RAIN_OF_TREATS at 11s, C++
-- (boss_headless_horseman.cpp:316-323) arm order: DoCastSelf(43344)
-- non-triggered, SetFlag(UNIT_FLAG_NOT_SELECTABLE),
-- instance-SetData(DATA_PREPARE_RESET) [strands — no instance-script
-- bridge], SummonGameObject(GO_PUMPKIN_SHRINE 186267 at 1776.27,
-- 1348.74, 20.4116, o 6.27281, 7 days) + RemoveGameObject [tracking
-- leg, no Lua-visible effect], SummonGameObject(GO_LOOSELY_TURNED_
-- SOIL 186314 at 1765.28, 1347.46, 17.5514, o 0.100363, 7 days) +
-- RemoveGameObject. 7_days = 604800s. QuaternionData rotation is
-- cosmetic and dropped (Go bridge uses identity rotation).
-- The head phase machine (boss_headless_horseman.cpp:189-335, PORTED
-- 2026-11-14 round109): the head's _phase lives in headPhase
-- (EventMap phase twin). The 42603 HP check has TWO delivery paths
-- sharing headPeriodicCheck: the RegisterAuraEvent below (live when a
-- real Spell.dbc ships 42603) and a 1s poll (the live path today —
-- the DBC period for 42603 is unverified in-repo, so the poll is a
-- documented 1s approximation). Double delivery is harmless: both
-- arms are phase-gated, and firing one removes the aura / cancels
-- the poll. The 42410/42399/43101/42401/42428 SpellScript dispatchers
-- collapse to direct DoAction calls (they are pure dispatchers).
local headPhase = {}

-- Forward declarations (tharon_ja / gal_darah idiom — plain
-- assignments below, never re-local-function).
local stopHeadPhase, handleStartPhase, hpCheckTick

-- C++ DoAction ACTION_HEAD_HP_67 (boss_headless_horseman.cpp:226-233):
-- phase 1 only: advance, remove the 42603 aura (also cancels the
-- poll), heal-body self-cast, STOP in 1s.
local function headHp67(creature)
    local guid = creature:GetGUIDLow()
    if (headPhase[guid] or PHASE_1) ~= PHASE_1 then
        return
    end
    headPhase[guid] = PHASE_2
    cancelKey(guid, "hpcheck")
    creature:RemoveAura(SPELL_HEAD_PERIODIC)
    creature:CastSpell(creature, SPELL_HEAL_BODY)
    schedule(guid, "stop_head_phase", 1000, function()
        stopHeadPhase(creature, guid)
    end)
end

-- C++ DoAction ACTION_HEAD_HP_34 (:234-243): phase 2 only: advance,
-- remove the 42603 aura, pause + heal-body self-casts (heal
-- triggered), STOP in 1s.
local function headHp34(creature)
    local guid = creature:GetGUIDLow()
    if (headPhase[guid] or PHASE_1) ~= PHASE_2 then
        return
    end
    headPhase[guid] = PHASE_3
    cancelKey(guid, "hpcheck")
    creature:RemoveAura(SPELL_HEAD_PERIODIC)
    creature:CastSpell(creature, SPELL_HEAD_PAUSE)
    creature:CastSpell(creature, SPELL_HEAL_BODY, true)
    schedule(guid, "stop_head_phase", 1000, function()
        stopHeadPhase(creature, guid)
    end)
end

-- C++ spell_headless_horseman_head_periodic::HandleHPCheck (:869-878):
-- 34 first, then 67 (else-if); HealthBelowPct is strict.
local function headPeriodicCheck(creature)
    local pct = creature:GetHealthPct()
    if pct < 34 then
        headHp34(creature)
    elseif pct < 67 then
        headHp67(creature)
    end
end

-- 1s poll twin of the 42603 periodic ticks. Re-schedules only while
-- a head phase is live (phase 1/2, boss alive and mid head-phase);
-- stops on evade/death/reset, where the boss phase var is back to 1.
hpCheckTick = function(creature, guid)
    if creature:IsDead() then
        return
    end
    local boss = creature:FindNearestCreature(ENTRY_HEADLESS_HORSEMAN, 200)
    if boss == nil or boss:IsDead() then
        return
    end
    if (horsemanPhase[boss:GetGUIDLow()] or PHASE_1) == PHASE_1 then
        return
    end
    local before = headPhase[guid] or PHASE_1
    if before == PHASE_3 then
        return
    end
    headPeriodicCheck(creature)
    if (headPhase[guid] or PHASE_1) == before then
        schedule(guid, "hpcheck", 1000, function()
            hpCheckTick(creature, guid)
        end)
    end
end

-- C++ HandleStartPhase (boss_headless_horseman.cpp:260-290): stun
-- removal, visual self-cast (triggered), NOT_SELECTABLE removal,
-- MoveRandom(30); phase 1: 42603 self-cast (triggered); phase 2:
-- 42603 + speaks + request-body (triggered) + Talk(SAY_REQUEST_BODY);
-- phase 3: speaks + request-body + Talk. The 43101 cast collapses
-- to horseman:DoAction(5) (pure dispatcher). The poll starts in
-- phases 1/2 only (phase 3 has no HP arm left).
handleStartPhase = function(creature, guid)
    creature:RemoveAura(SPELL_HEAD_STUN)
    creature:CastSpell(creature, SPELL_HEAD_VISUAL, true)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:MoveRandom(30)
    local phase = headPhase[guid] or PHASE_1
    if phase == PHASE_1 then
        creature:CastSpell(creature, SPELL_HEAD_PERIODIC, true)
    elseif phase == PHASE_2 then
        creature:CastSpell(creature, SPELL_HEAD_PERIODIC, true)
        creature:CastSpell(creature, SPELL_SPEAKS, true)
        local horseman = creature:FindNearestCreature(ENTRY_HEADLESS_HORSEMAN, 200)
        if horseman ~= nil then
            horseman:DoAction(ACTION_HORSEMAN_REQUEST_BODY)
        end
        creature:Talk(SAY_HEAD_REQUEST_BODY)
    elseif phase == PHASE_3 then
        creature:CastSpell(creature, SPELL_SPEAKS, true)
        local horseman = creature:FindNearestCreature(ENTRY_HEADLESS_HORSEMAN, 200)
        if horseman ~= nil then
            horseman:DoAction(ACTION_HORSEMAN_REQUEST_BODY)
        end
        creature:Talk(SAY_HEAD_REQUEST_BODY)
    end
    if phase == PHASE_1 or phase == PHASE_2 then
        schedule(guid, "hpcheck", 1000, function()
            hpCheckTick(creature, guid)
        end)
    end
end

-- C++ EVENT_STOP_HEAD_PHASE (boss_headless_horseman.cpp:308-317):
-- stun self-cast, visual removal, DoCast(horseman, 42401, triggered)
-- collapses to horseman:DoAction(6), RemoveAllAttackers, motion
-- Clear (StopMoving twin), NOT_SELECTABLE flag.
stopHeadPhase = function(creature, guid)
    cancelKey(guid, "hpcheck")
    creature:CastSpell(creature, SPELL_HEAD_STUN)
    creature:RemoveAura(SPELL_HEAD_VISUAL)
    local horseman = creature:FindNearestCreature(ENTRY_HEADLESS_HORSEMAN, 200)
    if horseman ~= nil then
        horseman:DoAction(ACTION_HEAD_RETURN_TO_BODY)
    end
    creature:RemoveAllAttackers()
    creature:StopMoving()
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end

-- C++ npc_headless_horseman_head::DoAction (boss_headless_horseman.cpp:
-- 214-245) on ev38. START_HEAD_PHASE schedules the 1s delayed start.
local function onHeadDoAction(_, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_HEAD_START_HEAD_PHASE then
        schedule(guid, "start_head_delayed", 1000, function()
            handleStartPhase(creature, guid)
        end)
    elseif action == ACTION_HEAD_HP_67 then
        headHp67(creature)
    elseif action == ACTION_HEAD_HP_34 then
        headHp34(creature)
    end
end

local function onHeadDied(_, creature)
    local guid = creature:GetGUIDLow()
    -- C++ JustDied (boss_headless_horseman.cpp:296-300): phase DEAD,
    -- self-cast 42428 (collapses to horseman:DoAction(4) — the 42428
    -- SpellScript is a pure dispatcher; the pumpkin KillSelf leg
    -- rides the unseeded 52236 aura and stays out), then the 11s
    -- raintreats timer. A pending STOP can no longer fire (dead
    -- creatures don't tick UpdateAI), so it is cancelled with the poll.
    cancelKey(guid, "hpcheck")
    cancelKey(guid, "stop_head_phase")
    headPhase[guid] = HEAD_PHASE_DEAD
    local horseman = creature:FindNearestCreature(ENTRY_HEADLESS_HORSEMAN, 200)
    if horseman ~= nil then
        horseman:DoAction(ACTION_HEAD_IS_DEAD)
    end
    schedule(guid, "raintreats", 11000, function()
        creature:CastSpell(creature, SPELL_RAIN_OF_TREATS)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SummonGameObject(GO_PUMPKIN_SHRINE, 1776.27, 1348.74, 20.4116, 6.27281, 604800)
        creature:SummonGameObject(GO_LOOSELY_TURNED_SOIL, 1765.28, 1347.46, 17.5514, 0.100363, 604800)
    end)
end

-- C++ npc_headless_horseman_head::JustAppeared -> HandleInitialSetup
-- (boss_headless_horseman.cpp:194): non-triggered DoCastSelf(42408).
local function onHeadSpawn(_, creature)
    creature:SetDisplayId(creature:GetTemplateModelId(1))
    creature:CastSpell(creature, SPELL_HEAD_STUN)
end

-- C++ npc_headless_horseman_head::JustReachedHome
-- (boss_headless_horseman.cpp:206-214): _events.Reset + MotionMaster
-- Clear have no Lua counterpart with live effect (the head carries no Lua
-- timers or motion; its only schedule is the post-death raintreats, which
-- cannot be pending on a live home-arrival) + HandleInitialSetup re-cast
-- (non-triggered HEAD_STUN 42408, same as onHeadSpawn) + SetFlag
-- NOT_SELECTABLE; PassiveAI::JustReachedHome is the base no-op. The flag
-- clear rides the unported head-phase machine (HandleStartPhase), the same
-- standing limitation as the raintreats set above.
local function onHeadReachHome(_, creature)
    local guid = creature:GetGUIDLow()
    -- C++ JustReachedHome (:206-214): _events.Reset (pending head-phase
    -- timers die) + HandleInitialSetup (_phase back to 1).
    cancelKey(guid, "hpcheck")
    cancelKey(guid, "stop_head_phase")
    headPhase[guid] = PHASE_1
    creature:CastSpell(creature, SPELL_HEAD_STUN)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end

RegisterCreatureEvent(ENTRY_HORSEMAN_HEAD, 4, onHeadDied)
RegisterCreatureEvent(ENTRY_HORSEMAN_HEAD, 5, onHeadSpawn)
RegisterCreatureEvent(ENTRY_HORSEMAN_HEAD, 24, onHeadReachHome)
RegisterCreatureEvent(ENTRY_HORSEMAN_HEAD, 38, onHeadDoAction)

-- C++ spell_headless_horseman_head_periodic (boss_headless_horseman.cpp:
-- 862-878): the 42603 aura's periodic HP check, delivered by the
-- shared headPeriodicCheck above (see the poll note — the aura path
-- goes live when a real Spell.dbc ships 42603).
local function headPeriodicAura(event, aura, target)
    headPeriodicCheck(target)
end

RegisterAuraEvent(SPELL_HEAD_PERIODIC, 1, headPeriodicAura)

-- C++ npc_sir_thomas::Reset (boss_headless_horseman.cpp:708-720):
-- RemoveFlag(QUESTGIVER) + SetFlag(NOT_SELECTABLE) + self-cast
-- WISP_INVIS 42823, then a 9s one-shot self-cast WISP_FLIGHT_MISSILE
-- 42821. The bearer arrives via the ported boss onDied
-- SummonCreature(23904) leg, so Reset arms on events 5/23. The
-- SpellHit(42818) handler PORTED below (sirThomasSpellHit) — the
-- "SpellHit handlers not modeled" note is stale: ev14 fires live
-- (boss_ai.go:4267,4282) and all four legs are bridged.
local function sirThomasReset(event, creature)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:CastSpell(creature, SPELL_WISP_INVIS)
    local guid = creature:GetGUIDLow()
    schedule(guid, "wispFlight", 9000, function()
        creature:CastSpell(creature, SPELL_WISP_FLIGHT_MISSILE)
    end)
end

-- C++ npc_sir_thomas::SpellHit (boss_headless_horseman.cpp:720-730):
-- 42818 WISP_FLIGHT_PORT -> clear NOT_SELECTABLE, set QUESTGIVER,
-- remove WISP_INVIS 42823, self-cast C_GHOST_VISUAL 42575 (triggered).
local function sirThomasSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_WISP_FLIGHT_PORT then
        return
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_QUESTGIVER)
    creature:RemoveAura(SPELL_WISP_INVIS)
    creature:CastSpell(creature, SPELL_HEADLESS_HORSEMAN_C_GHOST_VISUAL, true)
end

local function sirThomasClear(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_SIR_THOMAS, 23, sirThomasReset)
RegisterCreatureEvent(ENTRY_SIR_THOMAS, 14, sirThomasSpellHit)
RegisterCreatureEvent(ENTRY_SIR_THOMAS, 5, sirThomasReset)
RegisterCreatureEvent(ENTRY_SIR_THOMAS, 4, sirThomasClear)

-- C++ npc_pulsing_pumpkin (23694, scarlet_monastery.h:71) +
-- npc_flame_bunny (23686, scarlet_monastery.h:73). See the header
-- notes above for the C++ arms and the Lua deltas.
local ENTRY_PULSING_PUMPKIN = 23694
local ENTRY_PUMPKIN_FIEND = 23545
local ENTRY_FLAME_BUNNY = 23686
local SPELL_PUMPKIN_LIFE_CYCLE = 42280
local SPELL_PUMPKIN_AURA = 42294
local SPELL_SPROUTING = 42281
local SPELL_SPROUT_BODY = 42285
local SPELL_SQUASH_SOUL = 42514
local SPELL_BURNING_COSMETIC_BASE = 43184
local SPELL_FIRE_SIZE_BIG = 43148
local UNIT_FLAG_IMMUNE_TO_NPC = 0x200 -- UnitDefines.h:133

local function onPumpkinSquash(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_SQUASH_SOUL)
    end
    schedule(guid, "squash", 10000, function() onPumpkinSquash(creature, guid) end)
end

local function onPumpkinSprout(creature, guid)
    creature:RemoveAura(SPELL_PUMPKIN_LIFE_CYCLE)
    creature:SetDisableGravity(false)
    creature:SetHover(false)
    creature:CastSpell(creature, SPELL_SPROUT_BODY, true)
    creature:UpdateEntry(ENTRY_PUMPKIN_FIEND)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_NPC)
    creature:SetReactState(2)
    creature:DoZoneInCombat()
end

-- C++ npc_pulsing_pumpkin::Reset (boss_headless_horseman.cpp:640-646):
-- REACT_PASSIVE + self-casts 42280 / 42294 / 42281, non-triggered.
-- The 42281 self-cast triggers the DoAction sprout chain, so both
-- chain timers start here.
local function pumpkinResetArms(creature)
    local guid = creature:GetGUIDLow()
    creature:SetReactState(0)
    creature:CastSpell(creature, SPELL_PUMPKIN_LIFE_CYCLE)
    creature:CastSpell(creature, SPELL_PUMPKIN_AURA)
    creature:CastSpell(creature, SPELL_SPROUTING)
    schedule(guid, "sprout", 1000, function() onPumpkinSprout(creature, guid) end)
    schedule(guid, "squash", 11000, function() onPumpkinSquash(creature, guid) end)
end

-- C++ npc_pulsing_pumpkin::InitializeAI (boss_headless_horseman.cpp:634-639):
-- SetDisableGravity(true) + SetHover(true), then Reset arms on spawn.
local function onPumpkinSpawn(_, creature)
    creature:SetDisableGravity(true)
    creature:SetHover(true)
    pumpkinResetArms(creature)
end

local function onPumpkinReset(_, creature)
    pumpkinResetArms(creature)
end

local function onPumpkinDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_PULSING_PUMPKIN, 5, onPumpkinSpawn)
RegisterCreatureEvent(ENTRY_PULSING_PUMPKIN, 23, onPumpkinReset)
RegisterCreatureEvent(ENTRY_PULSING_PUMPKIN, 4, onPumpkinDied)

-- C++ npc_flame_bunny::Reset (boss_headless_horseman.cpp:677-688):
-- 1s -> self-cast 43184; 3s -> self-cast 43148, repeated while the
-- repeat counter is < 2 (3 casts total, 2.2s apart).
local function onBunnyGrow(creature, guid, count)
    creature:CastSpell(creature, SPELL_FIRE_SIZE_BIG)
    if count < 3 then
        schedule(guid, "grow", 2200, function() onBunnyGrow(creature, guid, count + 1) end)
    end
end

local function bunnyResetArms(creature)
    local guid = creature:GetGUIDLow()
    schedule(guid, "burn", 1000, function()
        creature:CastSpell(creature, SPELL_BURNING_COSMETIC_BASE)
    end)
    schedule(guid, "grow", 3000, function() onBunnyGrow(creature, guid, 1) end)
end

-- C++ npc_flame_bunny ctor (boss_headless_horseman.cpp:670-673):
-- SetDisplayId(Modelid2), then Reset arms on spawn.
local function onBunnySpawn(_, creature)
    creature:SetDisplayId(creature:GetTemplateModelId(2))
    bunnyResetArms(creature)
end

local function onBunnyReset(_, creature)
    bunnyResetArms(creature)
end

local function onBunnyDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_FLAME_BUNNY, 5, onBunnySpawn)
RegisterCreatureEvent(ENTRY_FLAME_BUNNY, 23, onBunnyReset)
RegisterCreatureEvent(ENTRY_FLAME_BUNNY, 4, onBunnyDied)

-- C++ spell_headless_horseman_maniacal_laugh::HandleSound
-- (boss_headless_horseman.cpp:819-822): each tick the aura target
-- plays SOUNDID_MANIACAL_LAUGH (11975) directly. No
-- PreventDefaultAction in C++, so the handler returns nothing and
-- the Go default trigger cast still fires.
local function maniacalLaughTick(event, aura, target)
    target:PlayDirectSound(SOUNDID_MANIACAL_LAUGH)
end

RegisterAuraEvent(SPELL_MANIACAL_LAUGH_AURA, 1, maniacalLaughTick)

-- C++ spell_summon_pumpkin_burst_delay::HandleText
-- (boss_headless_horseman.cpp:940-944): each tick the horseman
-- says SAY_SPROUTING_PUMPKINS (3). No PreventDefaultAction in C++,
-- so the handler returns nothing and the Go default trigger cast
-- still fires.
local function pumpkinBurstDelayTick(event, aura, target)
    target:Talk(SAY_SPROUTING_PUMPKINS)
end

RegisterAuraEvent(SPELL_SUMMON_PUMPKIN_BURST_DELAY, 1, pumpkinBurstDelayTick)

-- C++ spell_headless_horseman_summoning_rhyme_aura::PeriodicTick
-- (boss_headless_horseman.cpp:976-1005): the aura caster is the
-- horseman creature and the aura target is the player; tick 1/3/5/8
-- the caster Talks SAY_PLAYER_RISE/TIME/DEATH/DEMISE (2/3/4/5) to
-- the player (mandokir whisper precedent), tick 5 the player
-- self-casts SHAKE_SMALL (42910) triggered, tick 8 the player
-- self-casts SHAKE_MEDIUM (42909) triggered and the aura is removed.
-- No PreventDefaultAction in C++ (PERIODIC_DUMMY has no default
-- cast), so the handler returns nothing. PORTED tick 191 on the new
-- aura:GetTickNumber bridge.
local function summoningRhymeTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local tick = aura:GetTickNumber()
    if tick == 1 then
        caster:Talk(SAY_PLAYER_RISE, target)
    elseif tick == 3 then
        caster:Talk(SAY_PLAYER_TIME, target)
    elseif tick == 5 then
        caster:Talk(SAY_PLAYER_DEATH, target)
        target:CastSpell(target, SPELL_SUMMONING_RHYME_SHAKE_SMALL, true)
    elseif tick == 8 then
        caster:Talk(SAY_PLAYER_DEMISE, target)
        target:CastSpell(target, SPELL_SUMMONING_RHYME_SHAKE_MEDIUM, true)
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_SUMMONING_RHYME_AURA, 1, summoningRhymeTick)
