-- Coren Direbrew (Blackrock Depths, Grim Guzzler) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_coren_direbrew.cpp (boss_coren_direbrew —
-- BossAI combat scheduler via GetBlackrockDepthsAI, an instance-AI
-- retrieval wrapper).
-- Boss-script unit per eastern_kingdoms_script_loader.cpp order
-- (boss_coren_direbrew follows boss_tomb_of_seven; the BRD block
-- continues with the next boss group).
-- Entry (verifiable from the C++ sources): NPC_COREN = 23872 in
-- instance_blackrock_depths.cpp line 47 (the BRD instance creatures
-- enum). The creature_template ScriptName binding is DB-side (no TDB
-- in this workspace), but the entry itself is C++-verifiable so the
-- port is registered (npc_phalanx / boss_doomrel precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Eluna gossip events: 1 OnGossipHello,
-- 2 OnGossipSelect. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ DoAction(ACTION_START_FIGHT) event schedule +
-- UpdateAI event machine):
-- EVENT_SUMMON_MOLE_MACHINE: triggered self-cast Summon Mole Machine
-- Target Picker 47691, 15s init -> 15s loop (angerforge precedent —
-- self-cast timer, C++ casts with TRIGGERED_FULL_MASK + max-targets
-- mod; the picker/minion SpellScripts are unported but the self-cast
-- is a real engine spell).
-- EVENT_DIREBREW_DISARM: triggered self-cast Direbrew's Disarm
-- (precast) 47407, 20s init -> 20s loop (angerforge precedent —
-- self-cast timer; the disarm AuraScript itself is unported).
-- The timers schedule from OnEnterCombat(1) here; C++ schedules them
-- from DoAction(ACTION_START_FIGHT) which is only reachable through
-- the gossip fight option — see gossip notes below.
-- DamageTaken (PORTED below): 66% phase-two latch ->
-- SummonSister(NPC_ILSA_DIREBREW 26764) at the live position;
-- 33% phase-three latch -> SummonSister(NPC_URSULA_DIREBREW 26822)
-- — the SummonCreature + DoZoneInCombat bridges went live 2026-10-23
-- (retiring the old "no summon bridge" claim). The phase machine
-- collapses to per-guid one-shot flags (phases advance only through
-- these arms) with the C++ else-if ordering.
-- SummonedCreatureDies 1s sister-respawn re-arms are PORTED below
-- (event-21 dispatch bridged 2026-10-24, retiring the old "no bridge"
-- claim); the dead sister's entry re-arms a 1s timer that re-runs the
-- SummonSister twin.
-- Gossip (vael precedent — RegisterCreatureGossipEvent exists and
-- fires inline in gossip.go): OnGossipHello offers the fight and
-- apologize options; OnGossipSelect FIGHT closes the menu — the C++
-- arms (Talk(SAY_INSULT 3), SetImmuneToPC(false),
-- SetFaction(FACTION_GOBLIN_DARK_IRON_BAR_PATRON 736)) are PORTED below
-- (ingvar precedent: faction/flag bridges live); the antagonist
-- ACTION_ANTAGONIST_HOSTILE relay stays out (ev38 DoAction dispatch IS
-- bridged — the blocker is the summons.DoAction fan-out from Coren's
-- START_FIGHT arm (boss_coren_direbrew.cpp:175) to the unregistered
-- antagonist AI; no SummonList fan-out bridge).
-- Coren's own DoAction DoZoneInCombat arm (:172) is ported in
-- onGossipSelect below (bridge live 2026-10-23), so the fight still
-- starts when a player pulls. APOLOGIZE closes the menu C++-exact. The option/menu
-- texts are approximated — the real strings live in the DB
-- broadcast_text rows (vael precedent).
-- Unmodeled: intro phase (MoveInLineOfSight -> PHASE_INTRO event
-- machine, 3x NPC_ANTAGONIST 23795 summons at fixed positions,
-- SAY_INTRO/1/2 Talk arms, ACTION_ANTAGONIST_SAY_1/2/ HOSTILE relayed
-- via EntryCheckPredicate + DoAction) — the intro event machine + SummonList
-- fan-out have no bridge (SummonCreature itself is bridged; ev38 dispatch
-- is bridged — doomrel precedent for the Talk arms). The MIOS Talk(SAY_INTRO)
-- arm itself is additionally unreachable: Go's event-27 fire site is
-- the aggro-engage branch (hostile/attackable faction only,
-- creaturemotion.go:3522-3525), but the C++ intro fires pre-fight
-- while Coren is FACTION_FRIENDLY + ImmuneToPC — an event-27
-- handler would be dead code, so it stays documented-only.
-- npc_coren_direbrew_sisters (26764/26822) — PORTED in
-- npc_coren_direbrew_sisters.lua: JustEngagedWith self-cast PORT_TO_COREN
-- 52850 + control aura 47369/50278, then a 2s->4s SelectTarget(Random, 0,
-- 0.0f, false, true, -47331) mug chuck 50276, ported via the threat-list
-- pick (GetAITargets any-unit + HasAura(47331) exclusion);
-- the 47369 control-aura tick is PORTED in boss_coren_direbrew.lua
-- (tick 189, RegisterAuraEvent); the 50278 barreled tick is PORTED
-- below (tick 190, RegisterAuraEvent). The old "has no
-- SelectTarget bridge, so no lua file" claim is retracted.
-- Unmodeled: npc_direbrew_minion (faction latch + DoZoneInCombat + JustSummoned
-- instance-GUID latch — no faction/instance bridges); npc_direbrew_antagonist
-- (DoAction Talk/faction arms — the antagonist AI is unregistered, so its
-- ev38 receiver has no bearer; Talk/faction themselves are bridged).
-- Unmodeled: go_direbrew_mole_machine (GameObjectAI Reset -> UseDoorOrButton
-- + 50313 emerge + linked-trap GO activation — no GO Reset/Spawn event
-- (gossip/use bridges exist, but the machine fires from Reset, not use);
-- the GO surface also lacks CastSpell + GetLinkedTrap).
-- AuraScripts (tick 189): spell_send_mug_control_aura (47369)
-- PeriodicTick EFFECT_0 is PORTED below — triggered self-cast
-- 47370 each tick (muru precedent; bearer: the sisters self-cast
-- 47369 on engage, npc_coren_direbrew_sisters.lua). spell_direbrew_
-- disarm (47407) PeriodicTick EFFECT_1 is PORTED below — GROW
-- 47409 +1 stack, -1500ms duration per tick (target:GetAura +
-- aura SetStackAmount/SetDuration bridges live, lua_aura.go).
-- The same AuraScript's OnApply arm (casts 47409 + 47310) is
-- PORTED below too (tick 196, RegisterAuraEvent OnApply) — the
-- GROW latent is resolved.
-- Unmodeled: the remaining SpellScripts (47691 target picker,
-- 47370 send mug picker, 47344 request second mug) — no
-- SpellScript bridges (hellfire_peninsula precedent).
-- spell_barreled_control_aura (50278) PeriodicTick EFFECT_0 is
-- PORTED below (tick 190) — PreventDefaultAction + triggered
-- cast 47442 on nil target.
-- Unmodeled: JustDied LFG FinishDungeon(287) arm (sLFGMgr + group gates —
-- no LFG bridge).
-- Deviations from C++: the C++ UpdateAI pre-loop CASTING gate
-- (boss_coren_direbrew.cpp:230) is ported as a 500ms IsCasting defer
-- on the mole-machine, disarm, and sister-respawn arms (mal_ganis /
-- balinda idiom); the in-loop gate's (:278) same-tick co-expiry
-- suppression has no timer-arm model (independent callbacks, AV
-- precedent) — the only unmodeled sliver. The C++ scheduler is driven
-- from DoAction and drained by UpdateAI; the port schedules from
-- OnEnterCombat(1) and cancels on 2/4/23 (maiden convention).

local ENTRY_COREN = 23872

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100 -- UnitDefines.h (Unit.cpp SetImmuneToPC)
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local FACTION_GOBLIN_DARK_IRON_BAR_PATRON = 736 -- SharedDefines.h:249

local SPELL_MOLE_MACHINE_TARGET_PICKER = 47691
local SPELL_DIREBREW_DISARM_PRE_CAST = 47407
local SPELL_SEND_MUG_CONTROL_AURA = 47369 -- boss_coren_direbrew.cpp:74
local SPELL_SEND_MUG_TARGET_PICKER = 47370 -- boss_coren_direbrew.cpp:75
local SPELL_BARRELED_CONTROL_AURA = 50278 -- boss_coren_direbrew.cpp:539
local SPELL_BARRELED = 47442 -- boss_coren_direbrew.cpp:77
local SPELL_DIREBREW_DISARM_GROW = 47409 -- boss_coren_direbrew.cpp:72
local SPELL_DIREBREW_DISARM = 47310 -- boss_coren_direbrew.cpp:67

local NPC_ILSA_DIREBREW = 26764 -- boss_coren_direbrew.cpp:55
local NPC_URSULA_DIREBREW = 26822 -- boss_coren_direbrew.cpp:56
-- Lua SummonCreature summonType numbering is the C++ TempSummonType
-- minus 1 (mandokir precedent: C++ TEMPSUMMON_TIMED_DESPAWN_OUT_OF_
-- COMBAT 4 -> Lua 3), so C++ TEMPSUMMON_DEAD_DESPAWN 7 -> Lua 6.
local TEMPSUMMON_DEAD_DESPAWN = 6

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_FIGHT_TEXT = "I'll take that challenge!"
local GOSSIP_APOLOGIZE_TEXT = "Sorry, I'm too busy to fight."

local timers = {}
local sisterFlags = {} -- per-guid one-shot latches for the DamageTaken sister summons
local corenSisters = {} -- per-guid latched sister objects (boss_coren_direbrew.cpp:151/207/516 DespawnAll)

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

-- C++ EVENT_SUMMON_MOLE_MACHINE: triggered self-cast 47691; re-arm 15s.
-- Pre-loop CASTING gate (boss_coren_direbrew.cpp:230): a firing arm
-- that finds the creature casting re-schedules itself after 500ms
-- and consumes no re-arm (mal_ganis / balinda idiom; IsCasting live,
-- lua_creature_events.go:1950), so the arm stays due until the cast ends.
local function onSummonMoleMachine(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "molemachine", 500, function()
            onSummonMoleMachine(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_MOLE_MACHINE_TARGET_PICKER, true)
    schedule(guid, "molemachine", 15000, function()
        onSummonMoleMachine(creature, guid)
    end)
end

-- C++ EVENT_DIREBREW_DISARM: DoCastSelf(47407, true); re-arm 20s.
-- Same pre-loop gate as above.
local function onDirebrewDisarm(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "disarm", 500, function()
            onDirebrewDisarm(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DIREBREW_DISARM_PRE_CAST, true)
    schedule(guid, "disarm", 20000, function()
        onDirebrewDisarm(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; inherited base, no override in boss_coren_direbrew.cpp)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; inherited base, no override)
    local guid = creature:GetGUID()
    sisterFlags[guid] = nil
    schedule(guid, "molemachine", 15000, function()
        onSummonMoleMachine(creature, guid)
    end)
    schedule(guid, "disarm", 20000, function()
        onDirebrewDisarm(creature, guid)
    end)
end

local function despawnCorenSisters(guid)
    local sisters = corenSisters[guid]
    if sisters then
        for _, sister in ipairs(sisters) do
            if sister then
                sister:Despawn()
            end
        end
        corenSisters[guid] = nil
    end
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    sisterFlags[guid] = nil
    cancelTimers(guid)
    -- C++ EnterEvadeMode (boss_coren_direbrew.cpp:148-152): _EnterEvadeMode()
    -- + summons.DespawnAll() + _DespawnAtEvade(Seconds(10)); the sisters are
    -- Lua-summoned (unlike the antagonists, never summoned in Lua, and the
    -- mole machines, GO AI unmodeled), so they are latched and despawned
    -- here, ahead of Coren's own despawn (C++ order). Despawn fires event
    -- 20, not event 21, so no 1s sister-respawn re-arm follows.
    despawnCorenSisters(guid)
    creature:Despawn()
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    sisterFlags[guid] = nil
    cancelTimers(guid)
    -- C++ JustDied (boss_coren_direbrew.cpp:204-206) calls _JustDied(),
    -- whose summons.DespawnAll() (ScriptedCreature.cpp:513-516) drops the
    -- sisters; same latch as the evade path above.
    despawnCorenSisters(guid)
end

-- C++ SummonSister (boss_coren_direbrew.cpp:217-220): SummonCreature at
-- me->GetPosition() with TEMPSUMMON_DEAD_DESPAWN, then DoZoneInCombat
-- on the sister. Both bridges live (2026-10-23); creature:GetX()/GetY()/
-- GetZ()/GetO() read the live position via the object.go fallback.
local function summonSister(creature, entry)
    local sister = creature:SummonCreature(entry, creature:GetX(),
        creature:GetY(), creature:GetZ(), creature:GetO(),
        TEMPSUMMON_DEAD_DESPAWN)
    if sister then
        sister:DoZoneInCombat()
        local guid = creature:GetGUID()
        local list = corenSisters[guid]
        if not list then
            list = {}
            corenSisters[guid] = list
        end
        list[#list + 1] = sister
    end
end

local function onDamageTaken(event, creature, attacker, damage)
    -- C++ DamageTaken (boss_coren_direbrew.cpp:181-191): 66% one-shot
    -- phase-two latch -> SummonSister(NPC_ILSA_DIREBREW 26764); 33%
    -- one-shot phase-three latch -> SummonSister(NPC_URSULA_DIREBREW
    -- 26822). The C++ gates are HealthBelowPctDamaged(pct, damage) +
    -- IsInPhase (PHASE_ONE / PHASE_TWO); the phases advance only through
    -- these two arms, so per-guid one-shot flags are C++-exact, and the
    -- elseif preserves the C++ else-if ordering (one blow fires at most
    -- one latch). The SummonedCreatureDies 1s sister-respawn re-arms are
    -- ported in onSummonedCreatureDies below (event-21 bridge).
    -- Damage is never modified (thekal convention returns nothing when
    -- the hook does not alter it).
    local guid = creature:GetGUID()
    local flags = sisterFlags[guid]
    if not flags then
        flags = {}
        sisterFlags[guid] = flags
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if not flags.ilsa and health - damage < math.floor(maxHealth * 66 / 100) then
        flags.ilsa = true
        summonSister(creature, NPC_ILSA_DIREBREW)
    elseif not flags.ursula and health - damage < math.floor(maxHealth * 33 / 100) then
        flags.ursula = true
        summonSister(creature, NPC_URSULA_DIREBREW)
    end
end

local function onReset(event, creature)
    sisterFlags[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
    -- C++ Reset (boss_coren_direbrew.cpp:137-139) calls _Reset(), whose
    -- summons.DespawnAll() (ScriptedCreature.cpp:499-507) drops the sisters;
    -- Go fires 23 ahead of 2/4, so this also covers evade and death.
    despawnCorenSisters(creature:GetGUID())
    -- C++ Reset() (boss_coren_direbrew.cpp:138-141): SetImmuneToPC(true) +
    -- SetFaction(FACTION_FRIENDLY) — both bridged (ingvar precedent).
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    creature:SetFaction(FACTION_FRIENDLY)
end

local function onGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_FIGHT_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_APOLOGIZE_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN then
        return false
    end
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipClearMenu()
        player:GossipComplete()
        -- C++ GOSSIP_OPTION_FIGHT (boss_coren_direbrew.cpp:128 + DoAction
        -- ACTION_START_FIGHT :168-171): Talk(SAY_INSULT 3) +
        -- SetImmuneToPC(false) + SetFaction(FACTION_GOBLIN_DARK_IRON_BAR_PATRON)
        -- — all bridged (ingvar precedent); DoZoneInCombat ported below in
        -- C++ arm order (bridge live 2026-10-23); the antagonist
        -- ACTION_ANTAGONIST_HOSTILE relay stays out (ev38 dispatch IS bridged;
        -- the summons.DoAction fan-out to the unregistered antagonists has no bridge).
        creature:Talk(3)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:SetFaction(FACTION_GOBLIN_DARK_IRON_BAR_PATRON)
        creature:DoZoneInCombat() -- boss_coren_direbrew.cpp:172
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipClearMenu()
        player:GossipComplete()
        -- C++ GOSSIP_OPTION_APOLOGIZE: CloseGossipMenuFor — C++-exact.
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

-- C++ SummonedCreatureDies (boss_coren_direbrew.cpp:196-202) + the
-- EVENT_RESPAWN_ILSA / EVENT_RESPAWN_URSULA arms (:255-260): the dead
-- summon's entry re-arms a 1s timer that re-summons that sister via the
-- SummonSister twin. Fired by the engine event-21 dispatch
-- (fireCreatureDied -> summoner OwnerGUID link); C++ ScheduleEvent
-- re-arms an already-scheduled event, matching schedule()'s
-- replace-pending-key twin.
-- Respawn re-arm with the same pre-loop gate: while casting the
-- summon stays due and is retried after 500ms (re-checked each retry).
local function respawnSister(creature, guid, key, entry)
    if creature:IsCasting() then
        schedule(guid, key, 500, function()
            respawnSister(creature, guid, key, entry)
        end)
        return
    end
    summonSister(creature, entry)
end

local function onSummonedCreatureDies(event, creature, summon, killer)
    local guid = creature:GetGUID()
    local entry = summon:GetEntry()
    -- These re-arms sit inside the same gated C++ event loop
    -- (boss_coren_direbrew.cpp:230 pre-loop gate), so they defer
    -- while casting like the cast arms above.
    if entry == NPC_ILSA_DIREBREW then
        schedule(guid, "respawnIlsa", 1000, function()
            respawnSister(creature, guid, "respawnIlsa", NPC_ILSA_DIREBREW)
        end)
    elseif entry == NPC_URSULA_DIREBREW then
        schedule(guid, "respawnUrsula", 1000, function()
            respawnSister(creature, guid, "respawnUrsula", NPC_URSULA_DIREBREW)
        end)
    end
end

-- C++ spell_send_mug_control_aura::PeriodicTick
-- (boss_coren_direbrew.cpp:533-536): the aura target casts
-- SPELL_SEND_MUG_TARGET_PICKER (47370) triggered on itself each
-- tick; the aura itself (47369) is self-cast by the sisters on
-- engage (npc_coren_direbrew_sisters.lua), and the engine fires
-- this hook from the periodic tick (RegisterAuraEvent,
-- AuraEventOnPeriodic = 1; muru precedent). No PreventDefaultAction
-- in C++, so the handler returns nothing.
local function sendMugControlTick(event, aura, target)
    target:CastSpell(target, SPELL_SEND_MUG_TARGET_PICKER, true)
end

RegisterAuraEvent(SPELL_SEND_MUG_CONTROL_AURA, 1, sendMugControlTick)

-- C++ spell_direbrew_disarm::PeriodicTick
-- (boss_coren_direbrew.cpp:581-588): each tick the GROW aura
-- (47409) on the target gains one stack and loses 1500ms of
-- duration, C++ order preserved (SetStackAmount, then SetDuration
-- off the post-stack GetDuration). GetAura returns nil when GROW
-- is absent (the C++ null-check), so the tick is a no-op then.
-- GROW is applied by the same AuraScript's OnApply arm, PORTED
-- below (tick 196, RegisterAuraEvent OnApply).
local function direbrewDisarmTick(event, aura, target)
    local grow = target:GetAura(SPELL_DIREBREW_DISARM_GROW)
    if grow then
        grow:SetStackAmount(grow:GetStackAmount() + 1)
        grow:SetDuration(grow:GetDuration() - 1500)
    end
end

RegisterAuraEvent(SPELL_DIREBREW_DISARM_PRE_CAST, 1, direbrewDisarmTick)

-- C++ spell_direbrew_disarm::OnApply
-- (boss_coren_direbrew.cpp:573-577): on apply the target casts
-- SPELL_DIREBREW_DISARM_GROW (47409) triggered on itself, then
-- re-casts SPELL_DIREBREW_DISARM (47310) on itself, non-triggered.
-- The engine fires this hook from the aura-apply path
-- (RegisterAuraEvent, AuraEventOnApply = 2; tick-196 hook). No
-- PreventDefaultAction in C++, so the handler returns nothing.
-- The 47310 re-cast refreshes rather than re-firing: the hook
-- fires once per fresh application only (runtime.go contract), so
-- the re-cast cannot recurse. (C++ registers this arm with the
-- AuraEffectRemoveFn type — a TrinityCore quirk, no behavior.)
local function direbrewDisarmApply(event, aura, target)
    target:CastSpell(target, SPELL_DIREBREW_DISARM_GROW, true)
    target:CastSpell(target, SPELL_DIREBREW_DISARM, false)
end

RegisterAuraEvent(SPELL_DIREBREW_DISARM_PRE_CAST, 2, direbrewDisarmApply)

-- C++ spell_barreled_control_aura::PeriodicTick
-- (boss_coren_direbrew.cpp:544-547): PreventDefaultAction, then the
-- aura target casts SPELL_BARRELED (47442) triggered on nil target
-- each tick; the engine fires this hook from the periodic tick
-- (RegisterAuraEvent, AuraEventOnPeriodic = 1). The handler returns
-- true, the PreventDefaultAction twin, so the Go default trigger
-- cast does not also fire.
local function barreledControlTick(event, aura, target)
    target:CastSpell(nil, SPELL_BARRELED, true)
    return true
end

RegisterAuraEvent(SPELL_BARRELED_CONTROL_AURA, 1, barreledControlTick)

RegisterCreatureEvent(ENTRY_COREN, 21, onSummonedCreatureDies)
RegisterCreatureEvent(ENTRY_COREN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_COREN, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_COREN, 4, onDied)
RegisterCreatureEvent(ENTRY_COREN, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_COREN, 23, onReset)
RegisterCreatureGossipEvent(ENTRY_COREN, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_COREN, 2, onGossipSelect)
