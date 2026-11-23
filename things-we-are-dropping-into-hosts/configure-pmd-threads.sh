#!/bin/bash
die() {
    echo "" >&2
    exit 1
}

# Converts a cpulist (e.g. "2-5,8") into a hex mask. Bash integers are 64 bit
# and shifts wrap around, so the mask is built from 32-bit words to support
# hosts with more than 64 CPUs.
cpulist_to_mask() {
    local cpulist="$1"
    local -a words=()
    local -a ranges
    local range first last cpu i out
    local top=-1

    IFS=',' read -ra ranges <<< "$cpulist"
    for range in "${ranges[@]}"; do
        if [[ "$range" =~ ^([0-9]+)-([0-9]+)$ ]]; then
            first=${BASH_REMATCH[1]}
            last=${BASH_REMATCH[2]}
        elif [[ "$range" =~ ^[0-9]+$ ]]; then
            first=$range
            last=$range
        else
            die "Invalid CPU list entry: '$range'"
        fi
        for (( cpu=first; cpu<=last; cpu++ )); do
            i=$(( cpu / 32 ))
            words[i]=$(( ${words[i]:-0} | (1 << (cpu % 32)) ))
            (( i > top )) && top=$i
        done
    done

    [ "$top" -ge 0 ] || die "Empty CPU list"

    out=""
    for (( i=top; i>=0; i-- )); do
        if [ -z "$out" ]; then
            out=$(printf "%x" "${words[i]:-0}")
        else
            out+=$(printf "%08x" "${words[i]:-0}")
        fi
    done
    printf "0x%s" "$out"
}

[ -z "${OVS_DPDK_CPUS}" ] && die "OVS_DPDK_CPUS variable is not set in the ovs-vswitchd.service, leaving pmd-cpu-mask unconfigured"

mask=$(cpulist_to_mask ${OVS_DPDK_CPUS}) || die "Could not compute mask from '${OVS_DPDK_CPUS}'"
ovs-vsctl --timeout=10 --no-wait set Open_vSwitch . other_config:pmd-cpu-mask="$mask"
