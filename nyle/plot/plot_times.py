#!/usr/bin/env python3

import csv
import sys
import numpy as np
import matplotlib
import matplotlib.pyplot as plt
import argparse
from matplotlib.ticker import FuncFormatter
from pathlib import Path

def read_csv(csv_file):
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        read_count = 0
        for row in reader:
            read_count += 1
            if read_count > 1:
                print('we only accept csv files with two rows')
                return None
        if read_count == 0:
            print('empty csv file')
            return None
        return row



def process_own(data_dict):
    return process_data_dict(data_dict, lambda k: 'own' in k and '_wall_avg' in k)


def process_bunch(data_dict):
    return process_data_dict_tuple(data_dict, lambda k: 'bunch' in k and '_wall_avg' in k)


def process_all(data_dict):
    return process_data_dict(data_dict, lambda k: 'consensus_time' in k and '_wall_avg' in k)


def process_ownrx(data_dict):
    return process_data_dict(data_dict, lambda k: 'ownRx' in k and '_avg' in k)


def process_owntx(data_dict):
    return process_data_dict(data_dict, lambda k: 'ownTx' in k and '_avg' in k)


def process_ownrx_nr(data_dict):
    return process_data_dict(data_dict, lambda k: 'ownRxNr' in k and '_avg' in k)


def process_owntx_nr(data_dict):
    return process_data_dict(data_dict, lambda k: 'ownTxNr' in k and '_avg' in k)


def process_message_count(data_dict):
    return process_data_dict(data_dict, lambda k: 'messageCount' in k)


def process_data_dict(data_dict, matchf, comparison=lambda x, y: x < y):
    """
    general function for processing a dictionary
    comparison by default keeps the largest value
    """
    result = {}
    for k, v in data_dict.items():
        if not matchf(k):
            continue
        # strings are like <match>_site-10_<match>
        site = k.split('|')[1]
        data = float(v)
        if site in result:
            if comparison(result[site], data):
                result[site] = data
        else:
            result[site] = data
    return result


def process_data_dict_tuple(data_dict, matchf):
    """
    general function for processing a dictionary
    comparison by default keeps the largest value
    """
    result = {}
    for k, v in data_dict.items():
        if not matchf(k):
            continue
        # strings are like <match>_site-10_<match>
        site = k.split('|')[1]
        bunchNode = k.split('|')[2]

        if site not in result:
            result[site] = {}
        else:
            data = float(v)
            result[site][bunchNode] =  data

    return result


def plot_bar(data_all, data_own_ofall, data_own, data_bunch, data_rx, data_tx, data_rx_own, data_tx_own, data_rx_nr, data_tx_nr, data_rx_own_nr, data_tx_own_nr, labels, ylabel, yformatter=None, all_graphs=False):
    """
    """
    width = 5
    if all_graphs:
        width2 = 130
    else:
        width2=30
    fig, ax = plt.subplots()
    _, two = plt.subplots()
    _, three = plt.subplots()

    data_1 = []
    data_2 = []
    data_2_own = []
    data_3 = []
    data_plot_3 = []

    data_rx_plot = []
    data_rx_plot_own = []
    data_tx_plot = []
    data_tx_plot_own = []

    data_rx_plot_nr = []
    data_rx_plot_own_nr = []
    data_tx_plot_nr = []
    data_tx_plot_own_nr = []

    data_plot_3_own = []
    data_plot_own = []
    nodes = []
    labels = []
    ind = []

    max_bunch_size = 0
    bunch_maps = {}

    for x,y in zip(data_all.items(), data_own_ofall.items()):
        nodes.append(x[0])


    #print(nodes)

    for k in data_bunch:
        bunch_maps[k] = {}
        #print("aha")
        #print(data_bunch[k])
        for node_name in nodes:
            #print(node_name)
            if node_name not in data_bunch[k]:
                bunch_maps[k][node_name] = 0
            else:
                bunch_maps[k][node_name] = data_bunch[k][node_name]
    #print("bla")
    #print(bunch_maps)


    labels  = []

    for x,y,z,t,rx,tx,rxo,txo,rxnr,txnr,rxonr,txonr in zip(data_all.items(), data_own_ofall.items(), data_own.items(), bunch_maps.items(), data_rx.items(), data_tx.items(), data_rx_own.items(), data_tx_own.items(), data_rx_nr.items(), data_tx_nr.items(), data_rx_own_nr.items(), data_tx_own_nr.items()):
        data_1.append(x[1])
        data_2.append(y[1])
        data_2_own.append(z[1])
        data_3.append(t[1])
        data_rx_plot.append(rx[1])
        data_tx_plot.append(tx[1])
        data_rx_plot_own.append(rxo[1])
        data_tx_plot_own.append(txo[1])
        data_rx_plot_nr.append(rxnr[1])
        data_tx_plot_nr.append(txnr[1])
        data_rx_plot_own_nr.append(rxonr[1])
        data_tx_plot_own_nr.append(txonr[1])
        labels.append(x[0])

    index = 0
    for x in labels:
        data_plot_3.append([])
        for y in data_3:
            data_plot_3[index].append(y[x])
        index+=1


    #print("data plot")
    #print(data_plot_3)

    print(labels)

    if yformatter:
        ax.yaxis.set_major_formatter(yformatter)

    for x in range(len(labels)):
        ind.append(x * width2)

    #labels = np.arange(len(labels))

    m = [(lambda x: x.split('_')[1])(x) for x in labels]
    labels = m

    ind = np.array(ind)
    #print(ind)

    #print(data_1)
    #print(data_2)

    #print(len(data_2))
    #print(len(data_plot_3[0]))


    #rects_unopt = ax.bar(ind+width, data_1, width, edgecolor='b', hatch='/', fill=False, yerr=stds_unopt)
    rects_all = ax.bar(ind, data_1, width, edgecolor='b', hatch='/', fill=False)
    #rects_opt = ax.bar(ind, data_2, width, edgecolor='r', hatch='.', fill=False, yerr=stds_opt)
    rects_own_ofall = ax.bar(ind+width, data_2, width, edgecolor='r', hatch='.', fill=False)
    rects_own = ax.bar(ind + width*2, data_2_own, width, edgecolor='y', hatch='o', fill=False)

    rects_rx = two.bar(ind, data_rx_plot, width, label='golds', edgecolor='green', hatch='/', fill=False)
    rects_tx = two.bar(ind, data_tx_plot, width, label='golds', edgecolor='purple', hatch='+', fill=False, bottom=data_rx_plot)
    rects_rxo = two.bar(ind + width*2, data_rx_plot_own, width, label='silvers', color='black', edgecolor='green', hatch='/', fill=True)
    rects_txo = two.bar(ind + width*2, data_tx_plot_own, width, label='bronzes', color='black', edgecolor='purple', hatch='O', fill=True, bottom=data_rx_plot_own)

    rects_rx_nr = three.bar(ind, data_rx_plot_nr, width, label='golds', edgecolor='green', hatch='/', fill=False)
    rects_tx_nr = three.bar(ind, data_tx_plot_nr, width, label='golds', edgecolor='purple', hatch='+', fill=False,
                       bottom=data_rx_plot_nr)
    rects_rxo_nr = three.bar(ind + width * 2, data_rx_plot_own_nr, width, label='silvers', color='black', edgecolor='green',
                        hatch='/', fill=True)
    rects_txo_nr = three.bar(ind + width * 2, data_tx_plot_own_nr, width, label='bronzes', color='black', edgecolor='purple',
                        hatch='O', fill=True, bottom=data_rx_plot_own_nr)



    if all_graphs:
        for x in range(20):
            ax.bar(ind+width*(3+x), data_plot_3[x], width, edgecolor='g', hatch='x', fill=False)

    print(labels)

    ax.yaxis.grid()
    ax.set_xticks(ind + width / 2 )
    ax.set_ylabel(ylabel)
    ax.set_xlabel('Node index')

    ax.set_xticklabels(tuple(labels))
    ax.legend((rects_all[0], rects_own_ofall[0], rects_own[0]), ('All', 'Own of all', 'Only own'), loc='upper right')

    two.yaxis.grid()
    two.set_xticks(ind + width / 2)
    two.set_ylabel('Bytes')
    two.set_xlabel('Node index')

    two.set_xticklabels(tuple(labels))
    two.legend((rects_rx[0], rects_tx[0], rects_rxo[0], rects_txo[0]), ('Rx own - full load', 'Tx own - full load', 'Rx own - single', 'Tx own - single'), loc='upper right')

    three.yaxis.grid()
    three.set_xticks(ind + width / 2)
    three.set_ylabel('Nr messages')
    three.set_xlabel('Node index')

    three.set_xticklabels(tuple(labels))
    three.legend((rects_rx_nr[0], rects_tx_nr[0], rects_rxo_nr[0], rects_txo_nr[0]),
               ('Rx own - full load', 'Tx own - full load', 'Rx own - single', 'Tx own - single'), loc='upper right')


def read_levels(logfile):
    started = False
    results = {}
    with open(logfile, 'r') as f:
        for line in f:
            if 'main.AssignLevels' not in line or 'site' not in line:
                if started:
                    return results
                continue
            started = True
            # lines are like this:
            # 1 : (                          main.AssignLevels: 143) - site-3   2
            x = line.split(' - ')[1].split()
            site = x[0]
            lvl = int(x[1])
            if site in results:
                print("not possible")
                return None
            results[site] = lvl
    return results


def combine_dict(ds):
    """
    the list of dictionaries should have the same keys
    """
    combined = []
    for d in ds:
        for v in d.values():
            combined.append(v)
    return combined


if __name__ == '__main__':
    parser = argparse.ArgumentParser('Nyle results, log files must match csv files')
    parser.add_argument('--dirs', nargs='+', required=True, help='directory for the data files, one per experiment')
    parser.add_argument('--labels', nargs='+', required=True, help='labels of the experiments for the x-axis, should match dirs')
    parser.add_argument('--graph', choices=['time', 'message'], required=True, help='plot either the memory graph or the message count graph')
    args = parser.parse_args()

    data_own = {}
    data_own_2 = {}
    data_all = {}
    data_bunch = {}
    data_rx = {}
    data_tx = {}
    data_rx_own = {}
    data_tx_own = {}
    data_rx_nr = {}
    data_tx_nr = {}
    data_rx_own_nr = {}
    data_tx_own_nr = {}
    suffix="_fixed"

    for d in args.dirs:
        # optimised has 'cb'
        pathlist = Path(d).glob("nyle"+suffix+".csv")
        for p in pathlist:
            print(p)
            csv_data = read_csv(p)

            data_own = process_own(csv_data)
            data_all = process_all(csv_data)
            data_bunch = process_bunch(csv_data)
            data_rx = process_ownrx(csv_data)
            data_tx = process_owntx(csv_data)
            data_rx_nr = process_ownrx_nr(csv_data)
            data_tx_nr = process_owntx_nr(csv_data)

        pathlist = Path(d).glob("nyle_own"+suffix+".csv")
        for p in pathlist:
            print(p)
            csv_data = read_csv(p)
            data_own_2 = process_own(csv_data)
            data_rx_own = process_ownrx(csv_data)
            data_tx_own = process_owntx(csv_data)
            data_rx_own_nr = process_ownrx_nr(csv_data)
            data_tx_own_nr = process_owntx_nr(csv_data)

        print(data_own)
        print(data_all)
        print(data_bunch)

    matplotlib.rcParams.update({'font.size': 12})
    if args.graph == 'time':
        plot_bar(data_all, data_own, data_own_2, data_bunch, data_rx, data_tx, data_rx_own, data_tx_own, data_rx_nr, data_tx_nr, data_rx_own_nr, data_tx_own_nr, args.labels, 'Time (s)')
    else:
        plt.plot([1,2,3,4,5])

    plt.tight_layout()
    plt.show()
