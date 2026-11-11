-- Troll Volunteer (Durotar) --
-- Lua port of src/server/scripts/Kalimdor/zone_durotar.cpp
-- (npc_troll_volunteerAI). Zone-script unit; entries from the C++
-- Creatures enum: 40264 (NPC_VOLUNTEER_1), 40260 (NPC_VOLUNTEER_2).
-- The C++ CreatureScript binds one AI class to both entries, so both
-- register the same handlers (C++-exact).
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(creature, spell) = DoCastSelf.
-- Ported arms (C++ arm order):
-- InitializeAI (ev5 spawn): dead-or-ownerless gate, then Reset() +
-- mount-model pick + SetDisplayId(random trollmodel) + MoveFollow
-- (owner player, 5.0f, (rand_norm()+1)*PI/3*4 angle).
-- Reset (ev23): complete=false, AddAura VOLUNTEER_AURA 75076 +
-- MOUNTING_CHECK 75420, DoCastSelf PETACT_AURA 74071, REACT_PASSIVE,
-- Talk(SAY_VOLUNTEER_START 0).
-- SpellHit (ev14): once-only, spell AOE_TURNIN 75107 from URUZIN
-- 40253 -> DoCastSelf TURNIN 73953 + QUEST_CREDIT 75106, RemoveAura
-- MOUNTING_CHECK (RemoveAurasDueToSpell twin: single-spell removal),
-- Dismount, Talk(SAY_VOLUNTEER_END 1), MovePoint to the caster's
-- position (MoveTo twin).
-- MovementInform arrival leg PORTED (event 28, C++-exact):
-- POINT_URUZIN (4026400) arrival -> DespawnOrUnsummon.
-- Mount-check aura PORTED 2026-11-11 (RegisterAuraEvent 75420 ev1 tick,
-- zone_durotar.cpp:388-403): ownerless tick returns; owner mounted +
-- target not -> Mount(state mountModel); owner not mounted + target
-- mounted -> Dismount; RUN rate copied from the owner (GetSpeed(1)/7.0,
-- playerBaseMoveSpeed[1] = creatureBaseRunSpeed = 7.0).
-- Deliberate deviation: the WALK rate leg stays out (SetSpeedRate is
-- RUN-only, lua_creature_events.go:5641).
-- Melee is engine-driven in Go, like C++ DoMeleeAttackIfReady
-- (the volunteer is REACT_PASSIVE, so this never fires in practice).

local SPELL_VOLUNTEER_AURA = 75076
local SPELL_PETACT_AURA = 74071
local SPELL_QUEST_CREDIT = 75106
local SPELL_MOUNTING_CHECK = 75420
local SPELL_TURNIN = 73953
local SPELL_AOE_TURNIN = 75107

local NPC_URUZIN = 40253

local POINT_MOTION_TYPE = 8
local POINT_URUZIN = 4026400 -- zone_durotar.cpp:94

local REACT_PASSIVE = 0

local MOVE_RUN = 1
local BASE_RUN_SPEED = 7.0

local VOLUNTEER_ENTRIES = { 40264, 40260 }

local MOUNT_MODELS = { 6471, 6473, 6469, 6472 }

local TROLL_MODELS = {
    11665, 11734, 11750, 12037, 12038, 12042, 12049, 12849, 13529, 14759,
    15570, 15701, 15702, 1882, 1897, 1976, 2025, 27286, 2734, 2735,
    4084, 4085, 4087, 4089, 4231, 4357, 4358, 4360, 4361, 4362,
    4363, 4370, 4532, 4537, 4540, 4610, 6839, 7037, 9767, 9768,
}

local volunteerState = {}

local function stateFor(guid)
    local state = volunteerState[guid]
    if state == nil then
        state = { complete = false, mountModel = 0 }
        volunteerState[guid] = state
    end
    return state
end

local function volunteerReset(_, creature)
    local state = stateFor(creature:GetGUIDLow())
    state.complete = false
    creature:AddAura(SPELL_VOLUNTEER_AURA)
    creature:AddAura(SPELL_MOUNTING_CHECK)
    creature:CastSpell(creature, SPELL_PETACT_AURA)
    creature:SetReactState(REACT_PASSIVE)
    creature:Talk(0)
end

local function volunteerSpawn(_, creature)
    -- C++ InitializeAI gate: dead or ownerless -> no Reset, no follow.
    if creature:IsDead() then
        return
    end
    local owner = creature:GetOwner()
    if owner == nil then
        return
    end
    volunteerReset(_, creature)
    local state = stateFor(creature:GetGUIDLow())
    state.mountModel = MOUNT_MODELS[math.random(1, 4)]
    creature:SetDisplayId(TROLL_MODELS[math.random(1, 40)])
    creature:MoveFollow(owner, 5.0, (math.random() + 1.0) * math.pi / 3.0 * 4.0)
end

local function volunteerSpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_AOE_TURNIN then
        return
    end
    if caster == nil or caster:GetEntry() ~= NPC_URUZIN then
        return
    end
    local state = stateFor(creature:GetGUIDLow())
    if state.complete then
        return
    end
    state.complete = true
    creature:CastSpell(creature, SPELL_TURNIN)
    creature:CastSpell(creature, SPELL_QUEST_CREDIT)
    creature:RemoveAura(SPELL_MOUNTING_CHECK)
    creature:Dismount()
    creature:Talk(1)
    creature:MoveTo(POINT_URUZIN, caster:GetX(), caster:GetY(), caster:GetZ())
end

-- C++ MovementInform (zone_durotar.cpp:336-343): POINT_URUZIN arrival
-- -> DespawnOrUnsummon.
local function volunteerMovementInform(_, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    if id == POINT_URUZIN then
        creature:DespawnOrUnsummon()
    end
end

for _, entry in ipairs(VOLUNTEER_ENTRIES) do
    RegisterCreatureEvent(entry, 5, volunteerSpawn)
    RegisterCreatureEvent(entry, 23, volunteerReset)
    RegisterCreatureEvent(entry, 14, volunteerSpellHit)
    RegisterCreatureEvent(entry, 28, volunteerMovementInform)
end

-- C++ spell_mount_check::HandleEffectPeriodic
-- (zone_durotar.cpp:388-403, registered OnEffectPeriodic EFFECT_0
-- SPELL_AURA_PERIODIC_DUMMY): the aura target is the volunteer; its
-- owner is the player (GetOwner returns the player object or nil).
-- Owner mounted + target not -> Mount(state mountModel, the
-- InitializeAI pick, C++-exact incl. the 0-model case); owner not
-- mounted + target mounted -> Dismount; RUN rate copied from the
-- owner every tick. C++ also copies the WALK rate — stays out, no
-- walk-rate setter bridge.
local function mountCheckTick(_, _, target)
    if target == nil then
        return
    end
    local owner = target:GetOwner()
    if owner == nil then
        return
    end
    if owner:IsMounted() and not target:IsMounted() then
        local state = stateFor(target:GetGUIDLow())
        target:Mount(state.mountModel)
    elseif not owner:IsMounted() and target:IsMounted() then
        target:Dismount()
    end
    target:SetSpeedRate(MOVE_RUN, owner:GetSpeed(MOVE_RUN) / BASE_RUN_SPEED)
end

RegisterAuraEvent(SPELL_MOUNTING_CHECK, 1, mountCheckTick)
